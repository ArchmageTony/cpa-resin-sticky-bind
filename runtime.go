package main

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sync"
	"time"

	"github.com/ArchmageTony/cpa-resin-sticky-bind/internal/stickybind"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

type hostAdapter struct{}

func (hostAdapter) ListAuths() ([]stickybind.HostAuthEntry, error) {
	raw, err := callHostResult(pluginabi.MethodHostAuthList, map[string]any{})
	if err != nil {
		return nil, err
	}
	var resp struct {
		Files []stickybind.HostAuthEntry `json:"files"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode host.auth.list: %w", err)
	}
	return resp.Files, nil
}

func (hostAdapter) GetAuth(authIndex string) (stickybind.AuthGetResult, error) {
	raw, err := callHostResult(pluginabi.MethodHostAuthGet, pluginapi.HostAuthGetRequest{AuthIndex: authIndex})
	if err != nil {
		return stickybind.AuthGetResult{}, err
	}
	var resp stickybind.AuthGetResult
	if err := json.Unmarshal(raw, &resp); err != nil {
		return stickybind.AuthGetResult{}, fmt.Errorf("decode host.auth.get: %w", err)
	}
	return resp, nil
}

func (hostAdapter) SaveAuth(name string, rawJSON json.RawMessage) error {
	_, err := callHostResult(pluginabi.MethodHostAuthSave, pluginapi.HostAuthSaveRequest{
		Name: name,
		JSON: rawJSON,
	})
	return err
}

func (hostAdapter) Log(level, message string) {
	hostLog(level, message)
}

type runtime struct {
	mu     sync.Mutex
	cfg    stickybind.Config
	stopCh chan struct{}
	doneCh chan struct{}
	// id identifies this loaded plugin instance (one per .so load).
	id string
}

var (
	rtOnce sync.Once
	rtInst *runtime
)

func getRuntime() *runtime {
	rtOnce.Do(func() {
		rtInst = &runtime{
			cfg: configDefaults().Normalize(),
			id:  stickybind.NewInstanceID(),
		}
		hostLog("info", fmt.Sprintf("sticky-bind instance started id=%s version=%s", rtInst.id, pluginVer))
	})
	return rtInst
}

func (r *runtime) ApplyConfig(cfg stickybind.Config) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cfg = cfg.Normalize()
	r.restartLocked()
}

func (r *runtime) Config() stickybind.Config {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cfg
}

func (r *runtime) restartLocked() {
	if r.stopCh != nil {
		close(r.stopCh)
		if r.doneCh != nil {
			<-r.doneCh
		}
		r.stopCh = nil
		r.doneCh = nil
	}
	if !r.cfg.Enabled {
		return
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	r.stopCh = stop
	r.doneCh = done
	cfg := r.cfg
	go r.loop(cfg, stop, done)
}

func (r *runtime) loop(cfg stickybind.Config, stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	b := &stickybind.Binder{
		Cfg:    cfg,
		Host:   hostAdapter{},
		Getenv: os.Getenv,
	}
	lease := stickybind.Lease{Path: cfg.LeaseFile, ID: r.id}
	if err := lease.Claim(); err != nil {
		hostLog("warn", fmt.Sprintf("sticky-bind lease claim failed id=%s path=%s err=%v (continuing without single-instance guard)", r.id, cfg.LeaseFile, err))
	} else {
		hostLog("info", fmt.Sprintf("sticky-bind lease claimed id=%s path=%s", r.id, cfg.LeaseFile))
	}
	// run returns false when this instance lost the lease and must stop.
	run := func() bool {
		if held, owner := lease.Held(); !held {
			hostLog("warn", fmt.Sprintf("sticky-bind instance id=%s version=%s superseded by id=%s; stopping sync loop", r.id, pluginVer, owner))
			return false
		}
		wrote, skipped, unchanged, failed, err := b.SyncOnce()
		if err != nil {
			hostLog("error", "sticky-bind sync error: "+err.Error())
			return true
		}
		hostLog("info", fmt.Sprintf("sticky-bind sync id=%s wrote=%d skipped=%d unchanged=%d failed=%d", r.id, wrote, skipped, unchanged, failed))
		return true
	}
	// initial pass shortly after start
	select {
	case <-stop:
		return
	case <-time.After(2 * time.Second):
		if !run() {
			return
		}
	}
	ticker := time.NewTicker(time.Duration(cfg.SyncIntervalSeconds) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			// pick latest config each tick
			latest := r.Config()
			if !latest.Enabled {
				continue
			}
			if !reflect.DeepEqual(latest, b.Cfg) {
				// new targets must not be mistaken for external reverts
				b.ResetState()
			}
			b.Cfg = latest
			if !run() {
				return
			}
		}
	}
}

func (r *runtime) Shutdown() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopCh != nil {
		close(r.stopCh)
		if r.doneCh != nil {
			<-r.doneCh
		}
		r.stopCh = nil
		r.doneCh = nil
	}
}
