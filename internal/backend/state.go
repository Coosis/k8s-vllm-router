package backend

import (
	"sync/atomic"

	"github.com/Coosis/k8s-vllm-router/internal/ewma"
)

type BackendState struct {
	Endpoint Endpoint

	Inflight                  *atomic.Int64
	Healthy                   *atomic.Bool
	ConsecutiveHealthFailures *atomic.Uint64

	RequestsTotal *atomic.Uint64
	ErrorsTotal   *atomic.Uint64
	SelectedTotal *atomic.Uint64

	LatencyEWMA *ewma.Value
	ErrorEWMA   *ewma.Value
}

func NewBackendState(endpoint Endpoint) *BackendState {
	inflight := &atomic.Int64{}
	inflight.Store(0)
	healthy := &atomic.Bool{}
	healthy.Store(false)
	healthFailures := &atomic.Uint64{}
	healthFailures.Store(0)
	request := &atomic.Uint64{}
	request.Store(0)
	errors := &atomic.Uint64{}
	errors.Store(0)
	selected := &atomic.Uint64{}
	selected.Store(0)
	return &BackendState{
		Endpoint: endpoint,

		Inflight:                  inflight,
		Healthy:                   healthy,
		ConsecutiveHealthFailures: healthFailures,

		RequestsTotal: request,
		ErrorsTotal:   errors,
		SelectedTotal: selected,

		LatencyEWMA: ewma.NewEWMA(),
		ErrorEWMA:   ewma.NewEWMA(),
	}
}

type BackendSnapshot struct {
	List []*BackendState
	ByID map[string]*BackendState
}

func NewBackendSnapshot(states []*BackendState) *BackendSnapshot {
	byID := make(map[string]*BackendState, len(states))
	list := make([]*BackendState, 0, len(states))
	for _, state := range states {
		if state == nil {
			continue
		}
		list = append(list, state)
		byID[state.Endpoint.ID] = state
	}
	return &BackendSnapshot{
		List: list,
		ByID: byID,
	}
}
