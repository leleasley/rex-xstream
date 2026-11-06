package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/leleasley/rex-xstream/internal/config"
	"github.com/leleasley/rex-xstream/internal/epg"
	"github.com/leleasley/rex-xstream/internal/models"
	"github.com/leleasley/rex-xstream/internal/probe"
	"github.com/leleasley/rex-xstream/internal/rex"
	"github.com/leleasley/rex-xstream/internal/store"
	"github.com/leleasley/rex-xstream/internal/xc"
)

type Syncer struct {
	store  *store.Store
	cfg    *config.Config
	rex    *rex.Client
	prober probe.Prober
	log    *slog.Logger

	mu      sync.Mutex
	running map[int64]bool
}

func New(st *store.Store, cfg *config.Config, rexClient *rex.Client, prober probe.Prober, logger *slog.Logger) *Syncer {
	return &Syncer{store: st, cfg: cfg, rex: rexClient, prober: prober, log: logger, running: map[int64]bool{}}
}

func (s *Syncer) Running(playlistID int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running[playlistID]
}

// Start launches a sync if one is not already running for this playlist.
func (s *Syncer) Start(ctx context.Context, playlistID int64) bool {
	s.mu.Lock()
	if s.running[playlistID] {
		s.mu.Unlock()
		return false
	}
	s.running[playlistID] = true
	s.mu.Unlock()
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				s.log.Error("sync panic", "playlist", playlistID, "panic", recovered)
			}
			s.mu.Lock()
			delete(s.running, playlistID)
			s.mu.Unlock()
		}()
		if err := s.Run(context.WithoutCancel(ctx), playlistID); err != nil {
			s.log.Error("sync failed", "playlist", playlistID, "error", err)
		}
	}()
	return true
}

func (s *Syncer) StartAll(ctx context.Context) {
	playlists, err := s.store.EnabledPlaylists(ctx)
	if err != nil {
		s.log.Error("list enabled playlists", "error", err)
		return
	}
	for _, playlist := range playlists {
		s.Start(ctx, playlist.ID)
	}
}

// Test checks credentials and records provider information.
func (s *Syncer) Test(ctx context.Context, playlistID int64) (models.Playlist, error) {
	auth, err := s.store.PlaylistAuth(ctx, playlistID)
	if err != nil {
		return models.Playlist{}, err
	}
	client, err := xc.New(auth.BaseURL, auth.Username, auth.Password, s.cfg.RequestTimeout, s.cfg.AllowPrivateHosts)
	if err != nil {
		return models.Playlist{}, err
	}
	account, accountErr := client.Account(ctx)
	if accountErr != nil {
		// Some panels refuse a bare player_api.php request. A categories request
		// proves the credentials without assuming the account endpoint works.
		if _, fallbackErr := client.LiveCategories(ctx); fallbackErr != nil {
			_ = s.store.SetPlaylistTest(ctx, playlistID, "error", "{}", nil, 0)
			_ = s.store.SetPlaylistSyncResult(ctx, playlistID, "error", publicError(fallbackErr))
			return models.Playlist{}, fallbackErr
		}
		account = xc.Account{UserInfo: xc.UserInfo{Username: auth.Username, Status: "Active", Auth: 1}}
	}
	provider := models.Provider{
		ServerSoftware: account.ServerInfo.ServerSoftware,
		Timezone:       account.ServerInfo.Timezone,
		Protocol:       account.ServerInfo.ServerProtocol,
		Status:         account.UserInfo.Status,
		IsTrial:        account.UserInfo.IsTrial.String() == "1",
	}
	maxConnections, _ := strconv.Atoi(strings.TrimSpace(account.UserInfo.MaxConnections.String()))
	providerBody := marshalProvider(provider)
	expiry := account.Expiry()
	if err := s.store.SetPlaylistTest(ctx, playlistID, "connected", providerBody, expiry, maxConnections); err != nil {
		return models.Playlist{}, err
	}
	return s.store.GetPlaylist(ctx, playlistID)
}

func (s *Syncer) Run(ctx context.Context, playlistID int64) error {
	runID, err := s.store.StartSyncRun(ctx, playlistID)
	if err != nil {
		return err
	}
	counts := map[string]int{}
	phase := "catalog"
	var runErr error
	defer func() {
		status := "success"
		if runErr != nil {
			status = "error"
		}
		if err := s.store.FinishSyncRun(context.WithoutCancel(ctx), runID, status, phase, publicError(runErr), counts); err != nil {
			s.log.Error("finish sync run", "playlist", playlistID, "error", err)
		}
		if runErr != nil {
			_ = s.store.SetPlaylistSyncResult(context.WithoutCancel(ctx), playlistID, "error", publicError(runErr))
		}
		s.log.Info("sync finished", "playlist", playlistID, "status", status, "phase", phase)
	}()
	_ = s.store.SetPlaylistStatus(ctx, playlistID, "syncing")

	auth, err := s.store.PlaylistAuth(ctx, playlistID)
	if err != nil {
		runErr = err
		return err
	}
	client, err := xc.New(auth.BaseURL, auth.Username, auth.Password, s.cfg.RequestTimeout, s.cfg.AllowPrivateHosts)
	if err != nil {
		runErr = err
		return err
	}
	if _, err := s.Test(ctx, playlistID); err != nil {
		runErr = err
		return fmt.Errorf("connection test: %w", err)
	}

	liveCategories, err := client.LiveCategories(ctx)
	if err != nil {
		runErr = err
		return fmt.Errorf("live categories: %w", err)
	}
	if err := s.store.SyncCategories(ctx, playlistID, "live", mapCategories(liveCategories)); err != nil {
		runErr = err
		return err
	}
	movieCategories, err := client.VODCategories(ctx)
	if err != nil {
		runErr = err
		return fmt.Errorf("movie categories: %w", err)
	}
	if err := s.store.SyncCategories(ctx, playlistID, "movies", mapCategories(movieCategories)); err != nil {
		runErr = err
		return err
	}
	seriesCategories, err := client.SeriesCategories(ctx)
	if err != nil {
		runErr = err
		return fmt.Errorf("series categories: %w", err)
	}
	if err := s.store.SyncCategories(ctx, playlistID, "series", mapCategories(seriesCategories)); err != nil {
		runErr = err
		return err
	}

	liveStreams, err := client.LiveStreams(ctx)
	if err != nil {
		runErr = err
		return fmt.Errorf("live channels: %w", err)
	}
	channels := mapChannels(client, liveStreams, s.cfg.SyncMaxLive)
	if err := s.store.SyncChannels(ctx, playlistID, channels); err != nil {
		runErr = err
		return err
	}
	counts["channels"] = len(channels)

	movies, err := client.VODStreams(ctx)
	if err != nil {
		runErr = err
		return fmt.Errorf("movies: %w", err)
	}
	mappedMovies := dedupeMovies(mapMovies(client, movies, s.cfg.SyncMaxMovies))
	if err := s.store.SyncMovies(ctx, playlistID, mappedMovies); err != nil {
		runErr = err
		return err
	}
	counts["movies"] = len(mappedMovies)

	series, err := client.Series(ctx)
	if err != nil {
		runErr = err
		return fmt.Errorf("series: %w", err)
	}
	mappedSeries := dedupeSeries(mapSeries(series, s.cfg.SyncMaxSeries))
	if err := s.store.SyncSeries(ctx, playlistID, mappedSeries); err != nil {
		runErr = err
		return err
	}
	counts["series"] = len(mappedSeries)

	phase = "details"
	_ = s.store.SetSyncPhase(ctx, runID, phase)
	if err := s.syncSeriesDetails(ctx, client, playlistID); err != nil {
		s.log.Warn("series details incomplete", "playlist", playlistID, "error", err)
	}
	if episodes, err := s.countEpisodes(ctx, playlistID); err == nil {
		counts["episodes"] = episodes
	}

	phase = "epg"
	_ = s.store.SetSyncPhase(ctx, runID, phase)
	if err := s.syncEPG(ctx, client, playlistID); err != nil {
		s.log.Warn("EPG sync failed", "playlist", playlistID, "error", err)
	}
	if programmes, err := s.store.ProgrammeCount(ctx, playlistID); err == nil {
		counts["programmes"] = programmes
	}

	phase = "rex"
	_ = s.store.SetSyncPhase(ctx, runID, phase)
	if s.cfg.RexEnabled {
		if err := s.RegisterSource(ctx); err != nil {
			s.log.Warn("rex registration failed", "error", err)
		}
	}

	phase = "probes"
	_ = s.store.SetSyncPhase(ctx, runID, phase)
	probed, pushed, err := s.processProbes(ctx, playlistID)
	counts["probed"] = probed
	counts["pushed"] = pushed
	if err != nil {
		runErr = err
		return err
	}

	if s.cfg.RexEnabled {
		if err := s.removeInactive(ctx, playlistID); err != nil {
			s.log.Warn("rex removal failed", "playlist", playlistID, "error", err)
		}
	}

	phase = "done"
	s.log.Info("sync complete", "playlist", playlistID, "counts", counts)
	return nil
}

func (s *Syncer) syncSeriesDetails(ctx context.Context, client *xc.Client, playlistID int64) error {
	series, err := s.store.SeriesNeedingDetail(ctx, playlistID)
	if err != nil {
		return err
	}
	if len(series) == 0 {
		return nil
	}
	workers := s.cfg.SyncConcurrency
	if workers < 1 {
		workers = 1
	}
	jobs := make(chan models.Series)
	errs := make(chan error, len(series))
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range jobs {
				info, err := client.SeriesInfo(ctx, item.ProviderID)
				if err != nil {
					errs <- err
					continue
				}
				seasons, episodes := mapSeriesDetail(client, info)
				if err := s.store.SyncSeriesDetail(ctx, item.ID, seasons, episodes); err != nil {
					errs <- err
				}
			}
		}()
	}
	for _, item := range series {
		jobs <- item
	}
	close(jobs)
	wg.Wait()
	close(errs)
	var first error
	for err := range errs {
		if first == nil {
			first = err
		}
	}
	return first
}

func (s *Syncer) syncEPG(ctx context.Context, client *xc.Client, playlistID int64) error {
	epgIDs, err := s.store.ActiveEpgIDs(ctx, playlistID)
	if err != nil || len(epgIDs) == 0 {
		return err
	}
	body, err := client.XMLTV(ctx)
	if err != nil {
		return err
	}
	defer body.Close()
	from := time.Now().UTC().Add(-3 * time.Hour)
	to := time.Now().UTC().Add(time.Duration(s.cfg.EPGDays) * 24 * time.Hour)
	programmes, err := epg.Parse(body, from, to)
	if err != nil && len(programmes) == 0 {
		return err
	}
	allowed := map[string]bool{}
	for _, id := range epgIDs {
		allowed[id] = true
	}
	filtered := programmes[:0]
	for _, programme := range programmes {
		if allowed[programme.ChannelEPGID] {
			filtered = append(filtered, programme)
		}
	}
	return s.store.ReplaceProgrammes(ctx, playlistID, filtered)
}

func (s *Syncer) processProbes(ctx context.Context, playlistID int64) (int, int, error) {
	probed := 0
	pushed := 0
	failures := 0
	batch := s.cfg.ProbeWorkers * 4
	for {
		pending, err := s.store.PendingProbes(ctx, playlistID, batch)
		if err != nil {
			return probed, pushed, err
		}
		if len(pending) == 0 {
			break
		}
		var wg sync.WaitGroup
		results := make(chan struct {
			item ProbeResult
		}, len(pending))
		workers := s.cfg.ProbeWorkers
		if workers < 1 {
			workers = 1
		}
		jobs := make(chan store.ProbeItem)
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for item := range jobs {
					info, probeErr := s.prober.Probe(ctx, item.StreamURL, item.Live)
					probeText := ""
					media := ""
					if probeErr != nil {
						probeText = publicError(probeErr)
					} else {
						media = string(info)
					}
					_ = s.store.SaveProbe(ctx, item.Kind, item.ID, media, probeText)
					results <- struct{ item ProbeResult }{ProbeResult{Kind: item.Kind, ID: item.ID, OK: probeErr == nil}}
				}
			}()
		}
		for _, item := range pending {
			jobs <- item
		}
		close(jobs)
		wg.Wait()
		close(results)
		for result := range results {
			if result.item.OK {
				probed++
				failures = 0
			} else {
				failures++
			}
		}
		if s.cfg.RexEnabled {
			count, err := s.pushDirty(ctx, playlistID)
			if err != nil {
				return probed, pushed, err
			}
			pushed += count
		}
		if failures >= 40 {
			return probed, pushed, errors.New("too many failed probes, stopping this pass")
		}
	}
	if s.cfg.RexEnabled {
		count, err := s.pushDirty(ctx, playlistID)
		if err != nil {
			return probed, pushed, err
		}
		pushed += count
	}
	return probed, pushed, nil
}

type ProbeResult struct {
	Kind string
	ID   int64
	OK   bool
}

func (s *Syncer) pushDirty(ctx context.Context, playlistID int64) (int, error) {
	if !s.cfg.RexEnabled {
		return 0, nil
	}
	playlist, err := s.store.GetPlaylist(ctx, playlistID)
	if err != nil {
		return 0, err
	}
	total := 0
	pushed := 0
	for {
		movies, err := s.store.DirtyMovies(ctx, playlistID, s.cfg.RexPushBatch)
		if err != nil {
			return total, err
		}
		if len(movies) == 0 {
			break
		}
		items := make([]rex.Item, 0, len(movies))
		ids := make([]int64, 0, len(movies))
		rexIDs := make([]string, 0, len(movies))
		for _, movie := range movies {
			item, ok := rex.MovieItem(movie, rex.ItemContext{PlaylistName: playlist.Name, PublicURL: s.cfg.PublicURL, UseTMDB: s.cfg.RexSendTMDBIDs})
			if !ok {
				continue
			}
			items = append(items, item)
			ids = append(ids, movie.ID)
			rexIDs = append(rexIDs, item.ID)
		}
		if len(items) > 0 {
			if _, err := s.rex.Push(ctx, items); err != nil {
				return total, err
			}
			if err := s.store.MarkPushed(ctx, "movie", ids, rexIDs); err != nil {
				return total, err
			}
			total += len(items)
			pushed += len(items)
		}
		if len(movies) < s.cfg.RexPushBatch {
			break
		}
	}

	for {
		episodes, series, err := s.store.DirtyEpisodes(ctx, playlistID, s.cfg.RexPushBatch)
		if err != nil {
			return total, err
		}
		if len(episodes) == 0 {
			break
		}
		seriesByID := map[int64]models.Series{}
		for _, show := range series {
			seriesByID[show.ID] = show
		}
		items := make([]rex.Item, 0, len(episodes))
		ids := make([]int64, 0, len(episodes))
		rexIDs := make([]string, 0, len(episodes))
		for _, episode := range episodes {
			show, ok := seriesByID[episode.SeriesID]
			if !ok {
				continue
			}
			item, ok := rex.EpisodeItem(episode, show, rex.ItemContext{PlaylistName: playlist.Name, PublicURL: s.cfg.PublicURL, UseTMDB: s.cfg.RexSendTMDBIDs})
			if !ok {
				continue
			}
			items = append(items, item)
			ids = append(ids, episode.ID)
			rexIDs = append(rexIDs, item.ID)
		}
		if len(items) > 0 {
			if _, err := s.rex.Push(ctx, items); err != nil {
				return total, err
			}
			if err := s.store.MarkPushed(ctx, "episode", ids, rexIDs); err != nil {
				return total, err
			}
			total += len(items)
			pushed += len(items)
		}
		if len(episodes) < s.cfg.RexPushBatch {
			break
		}
	}

	for {
		channels, err := s.store.DirtyChannels(ctx, playlistID, s.cfg.RexPushBatch)
		if err != nil {
			return total, err
		}
		if len(channels) == 0 {
			break
		}
		epgIDs := make([]string, 0, len(channels))
		for _, channel := range channels {
			if channel.EPGID != "" {
				epgIDs = append(epgIDs, channel.EPGID)
			}
		}
		from := time.Now().UTC().Add(-3 * time.Hour).UnixMilli()
		to := time.Now().UTC().Add(time.Duration(s.cfg.EPGDays) * 24 * time.Hour).UnixMilli()
		guide, _ := s.store.Guide(ctx, playlistID, epgIDs, from, to)
		items := make([]rex.Item, 0, len(channels))
		ids := make([]int64, 0, len(channels))
		rexIDs := make([]string, 0, len(channels))
		for _, channel := range channels {
			item, ok := rex.LiveItem(channel, rex.ItemContext{PlaylistName: playlist.Name, PublicURL: s.cfg.PublicURL, Guide: guide, UseTMDB: s.cfg.RexSendTMDBIDs})
			if !ok {
				continue
			}
			items = append(items, item)
			ids = append(ids, channel.ID)
			rexIDs = append(rexIDs, item.ID)
		}
		if len(items) > 0 {
			if _, err := s.rex.Push(ctx, items); err != nil {
				return total, err
			}
			if err := s.store.MarkPushed(ctx, "channel", ids, rexIDs); err != nil {
				return total, err
			}
			total += len(items)
			pushed += len(items)
		}
		if len(channels) < s.cfg.RexPushBatch {
			break
		}
	}
	return pushed, nil
}

func (s *Syncer) removeInactive(ctx context.Context, playlistID int64) error {
	ids, err := s.store.InactiveRexIDs(ctx, playlistID)
	if err != nil || len(ids) == 0 {
		return err
	}
	if _, err := s.rex.Delete(ctx, ids); err != nil {
		return err
	}
	return s.store.ClearRexIDs(ctx, playlistID)
}

// RegisterSource tells rex which directories exist. The directory names are
// stable; renaming a playlist changes only the title.
func (s *Syncer) RegisterSource(ctx context.Context) error {
	if !s.cfg.RexEnabled {
		return nil
	}
	libraries, err := s.store.LibrariesForRex(ctx)
	if err != nil {
		return err
	}
	directories := make([]rex.Directory, 0, len(libraries))
	for _, lib := range libraries {
		switch lib.Kind {
		case "live":
			directories = append(directories, rex.Directory{Name: lib.Directory, Movies: lib.Title})
		case "movies":
			directories = append(directories, rex.Directory{Name: lib.Directory, Movies: lib.Title})
		case "series":
			directories = append(directories, rex.Directory{Name: lib.Directory, Shows: lib.Title})
		}
	}
	return s.rex.Register(ctx, s.cfg.PublicURL, directories)
}

// RenameLibraries updates the Plex library titles after a playlist rename.
// A registration alone does not rename an existing library.
func (s *Syncer) RenameLibraries(ctx context.Context, oldName, newName string) error {
	if !s.cfg.RexEnabled {
		return nil
	}
	return s.rex.RenameLibraries(ctx, []rex.Rename{
		{OldTitle: oldName + " Movies", NewTitle: newName + " Movies", Type: "movie"},
		{OldTitle: oldName + " Series", NewTitle: newName + " Series", Type: "show"},
	})
}

// DeletePlaylistItems removes everything a playlist pushed to rex before the
// playlist row is deleted.
func (s *Syncer) DeletePlaylistItems(ctx context.Context, playlistID int64) error {
	if !s.cfg.RexEnabled {
		return nil
	}
	ids, err := s.store.AllRexIDs(ctx, playlistID)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	_, err = s.rex.Delete(ctx, ids)
	return err
}

func (s *Syncer) countEpisodes(ctx context.Context, playlistID int64) (int, error) {
	counts, err := s.store.PlaylistCounts(ctx, playlistID)
	if err != nil {
		return 0, err
	}
	return counts["episodes"], nil
}

func marshalProvider(provider models.Provider) string {
	body, err := json.Marshal(provider)
	if err != nil {
		return "{}"
	}
	return string(body)
}

func mapCategories(items []xc.Category) []models.Category {
	out := make([]models.Category, 0, len(items))
	for _, item := range items {
		out = append(out, models.Category{ProviderID: item.CategoryID.String(), Name: item.CategoryName})
	}
	return out
}

func publicError(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if len(message) > 300 {
		message = message[:300]
	}
	return message
}
