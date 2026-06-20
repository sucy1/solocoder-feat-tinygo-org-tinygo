package filter

import (
	"encoding/binary"
	"errors"
	"math"
)

var (
	ErrInvalidWindowSize = errors.New("filter: invalid window size, must be between 2 and 64")
	ErrMisaligned        = errors.New("filter: data misaligned")
)

type MovingAverage struct {
	window []float64
	sum    float64
	index  int
	count  int
	size   int
}

func NewMovingAverage(windowSize int) (*MovingAverage, error) {
	if windowSize < 2 || windowSize > 64 {
		return nil, ErrInvalidWindowSize
	}
	return &MovingAverage{
		window: make([]float64, windowSize),
		size:   windowSize,
	}, nil
}

func (m *MovingAverage) Update(value float64) float64 {
	if m.count < m.size {
		m.count++
	} else {
		m.sum -= m.window[m.index]
	}
	m.window[m.index] = value
	m.sum += value
	m.index = (m.index + 1) % m.size
	return m.sum / float64(m.count)
}

func (m *MovingAverage) Value() float64 {
	if m.count == 0 {
		return 0
	}
	return m.sum / float64(m.count)
}

func (m *MovingAverage) Reset() {
	for i := range m.window {
		m.window[i] = 0
	}
	m.sum = 0
	m.index = 0
	m.count = 0
}

func (m *MovingAverage) Save() []byte {
	buf := make([]byte, 8+8+4+4+len(m.window)*8)
	binary.LittleEndian.PutUint64(buf[0:], math.Float64bits(m.sum))
	binary.LittleEndian.PutUint32(buf[8:], uint32(m.index))
	binary.LittleEndian.PutUint32(buf[12:], uint32(m.count))
	off := 16
	for _, v := range m.window {
		binary.LittleEndian.PutUint64(buf[off:], math.Float64bits(v))
		off += 8
	}
	return buf
}

func (m *MovingAverage) Load(data []byte) error {
	if len(data) < 16 {
		return ErrMisaligned
	}
	m.sum = math.Float64frombits(binary.LittleEndian.Uint64(data[0:]))
	m.index = int(binary.LittleEndian.Uint32(data[8:]))
	m.count = int(binary.LittleEndian.Uint32(data[12:]))
	expectedLen := 16 + len(m.window)*8
	if len(data) < expectedLen {
		return ErrMisaligned
	}
	off := 16
	for i := range m.window {
		m.window[i] = math.Float64frombits(binary.LittleEndian.Uint64(data[off:]))
		off += 8
	}
	if m.index < 0 || m.index >= m.size {
		m.index = 0
	}
	if m.count < 0 || m.count > m.size {
		m.count = 0
	}
	return nil
}

type Kalman struct {
	Q float64
	R float64

	x  float64
	p  float64
	k  float64
	initialized bool
}

func NewKalman(q, r float64) *Kalman {
	return &Kalman{
		Q: q,
		R: r,
	}
}

func (k *Kalman) Update(measurement float64) float64 {
	if !k.initialized {
		k.x = measurement
		k.p = 1.0
		k.initialized = true
		return measurement
	}

	k.p += k.Q

	k.k = k.p / (k.p + k.R)
	k.x += k.k * (measurement - k.x)
	k.p *= (1 - k.k)

	return k.x
}

func (k *Kalman) Value() float64 {
	return k.x
}

func (k *Kalman) Reset() {
	k.x = 0
	k.p = 0
	k.k = 0
	k.initialized = false
}

func (k *Kalman) Save() []byte {
	buf := make([]byte, 48)
	binary.LittleEndian.PutUint64(buf[0:], math.Float64bits(k.Q))
	binary.LittleEndian.PutUint64(buf[8:], math.Float64bits(k.R))
	binary.LittleEndian.PutUint64(buf[16:], math.Float64bits(k.x))
	binary.LittleEndian.PutUint64(buf[24:], math.Float64bits(k.p))
	binary.LittleEndian.PutUint64(buf[32:], math.Float64bits(k.k))
	if k.initialized {
		buf[40] = 1
	}
	return buf
}

func (k *Kalman) Load(data []byte) error {
	if len(data) < 48 {
		return ErrMisaligned
	}
	k.Q = math.Float64frombits(binary.LittleEndian.Uint64(data[0:]))
	k.R = math.Float64frombits(binary.LittleEndian.Uint64(data[8:]))
	k.x = math.Float64frombits(binary.LittleEndian.Uint64(data[16:]))
	k.p = math.Float64frombits(binary.LittleEndian.Uint64(data[24:]))
	k.k = math.Float64frombits(binary.LittleEndian.Uint64(data[32:]))
	k.initialized = data[40] != 0
	return nil
}
