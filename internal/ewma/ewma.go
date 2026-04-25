package ewma

import (
	"math"
	"sync/atomic"
)

const DefaultAlpha = 0.05

type Value struct {
	alpha float64
	bits  atomic.Uint64
}

func NewEWMA() *Value {
	return NewWithAlpha(DefaultAlpha)
}

func NewWithAlpha(alpha float64) *Value {
	if alpha <= 0 || alpha > 1 {
		alpha = DefaultAlpha
	}
	v := &Value{alpha: alpha}
	v.bits.Store(math.Float64bits(0))
	return v
}

func (v *Value) Observe(value float64) {
	for {
		oldBits := v.bits.Load()
		old := math.Float64frombits(oldBits)
		next := old*(1-v.alpha) + value*v.alpha
		if v.bits.CompareAndSwap(oldBits, math.Float64bits(next)) {
			return
		}
	}
}

func (v *Value) Get() float64 {
	return math.Float64frombits(v.bits.Load())
}
