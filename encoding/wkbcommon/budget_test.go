package wkbcommon

import (
	"errors"
	"math"
	"strconv"
	"testing"

	"github.com/alecthomas/assert/v2"
)

func TestReadBudgetCoordinates(t *testing.T) {
	t.Run("unlimited", func(t *testing.T) {
		b := NewReadBudget(ReadLimits{})
		assert.NoError(t, b.AddCoordinates(1_000_000, 4, ""))
		assert.Equal(t, int64(1_000_000), b.Coordinates())
	})

	t.Run("within limit", func(t *testing.T) {
		b := NewReadBudget(ReadLimits{MaxCoordinates: 10})
		assert.NoError(t, b.AddCoordinates(4, 2, ""))
		assert.NoError(t, b.AddCoordinates(6, 2, ""))
		assert.Equal(t, int64(10), b.Coordinates())
	})

	t.Run("exact limit is allowed", func(t *testing.T) {
		b := NewReadBudget(ReadLimits{MaxCoordinates: 10})
		assert.NoError(t, b.AddCoordinates(10, 3, ""))
	})

	t.Run("limit shared across calls", func(t *testing.T) {
		b := NewReadBudget(ReadLimits{MaxCoordinates: 10})
		assert.NoError(t, b.AddCoordinates(7, 2, "ring 0"))
		err := b.AddCoordinates(4, 2, "ring 1")
		var limitErr ErrLimitExceeded
		assert.True(t, errors.As(err, &limitErr))
		assert.Equal(t, ErrLimitExceeded{
			What:      "coordinates",
			Path:      "ring 1",
			Requested: 4,
			Used:      7,
			Limit:     10,
		}, limitErr)
		// The failed reservation must not consume any budget.
		assert.Equal(t, int64(7), b.Coordinates())
	})

	t.Run("rejected before any allocation can happen", func(t *testing.T) {
		b := NewReadBudget(ReadLimits{MaxCoordinates: 1})
		err := b.AddCoordinates(math.MaxUint32, 2, "")
		var limitErr ErrLimitExceeded
		assert.True(t, errors.As(err, &limitErr))
	})

	t.Run("stride multiplication overflow", func(t *testing.T) {
		b := NewReadBudget(ReadLimits{})
		err := b.AddCoordinates(math.MaxUint32, 4, "")
		if strconv.IntSize == 32 {
			// On 32-bit platforms the product overflows the platform int and
			// must be rejected before allocation.
			var overflowErr ErrIntegerOverflow
			assert.True(t, errors.As(err, &overflowErr))
			assert.Equal(t, "coordinate count", overflowErr.What)
		} else {
			// On 64-bit platforms a uint32 count times the maximum stride (4)
			// always fits, so there is nothing to reject when no budget is set.
			assert.NoError(t, err)
		}
	})

	t.Run("zero coordinates consume nothing", func(t *testing.T) {
		b := NewReadBudget(ReadLimits{MaxCoordinates: 0})
		assert.NoError(t, b.AddCoordinates(0, 2, ""))
		assert.Equal(t, int64(0), b.Coordinates())
	})
}

func TestReadBudgetMembers(t *testing.T) {
	t.Run("within limit", func(t *testing.T) {
		b := NewReadBudget(ReadLimits{MaxMembers: 5})
		assert.NoError(t, b.AddMembers(2, ""))
		assert.NoError(t, b.AddMembers(3, ""))
		assert.Equal(t, int64(5), b.Members())
	})

	t.Run("limit shared across levels", func(t *testing.T) {
		b := NewReadBudget(ReadLimits{MaxMembers: 5})
		assert.NoError(t, b.AddMembers(3, ""))
		err := b.AddMembers(3, "member 0")
		var limitErr ErrLimitExceeded
		assert.True(t, errors.As(err, &limitErr))
		assert.Equal(t, "members", limitErr.What)
		assert.Equal(t, "member 0", limitErr.Path)
		// The failed reservation must not consume any budget.
		assert.Equal(t, int64(3), b.Members())
	})

	t.Run("failed reservation consumes nothing", func(t *testing.T) {
		b := NewReadBudget(ReadLimits{MaxMembers: 2})
		assert.Error(t, b.AddMembers(3, ""))
		assert.Equal(t, int64(0), b.Members())
	})
}

func TestReadBudgetDepth(t *testing.T) {
	b := NewReadBudget(ReadLimits{MaxDepth: 2})

	release1, err := b.EnterGeometry("")
	assert.NoError(t, err)
	assert.Equal(t, 1, b.Depth())

	release2, err := b.EnterGeometry("member 0")
	assert.NoError(t, err)
	assert.Equal(t, 2, b.Depth())

	_, err = b.EnterGeometry("member 0/member 0")
	var limitErr ErrLimitExceeded
	assert.True(t, errors.As(err, &limitErr))
	assert.Equal(t, "depth", limitErr.What)
	assert.Equal(t, "member 0/member 0", limitErr.Path)
	// A rejected entry must not change the current depth.
	assert.Equal(t, 2, b.Depth())

	release2()
	assert.Equal(t, 1, b.Depth())
	release1()
	assert.Equal(t, 0, b.Depth())
}

func TestReadBudgetsAreIndependent(t *testing.T) {
	limits := ReadLimits{MaxCoordinates: 5, MaxMembers: 5, MaxDepth: 5}
	b1 := NewReadBudget(limits)
	b2 := NewReadBudget(limits)

	assert.NoError(t, b1.AddCoordinates(5, 2, ""))
	// b2 is unaffected by what b1 consumed.
	assert.NoError(t, b2.AddCoordinates(5, 2, ""))
	assert.Equal(t, int64(0), b2.Members())

	_, err := b1.EnterGeometry("")
	assert.NoError(t, err)
	assert.Equal(t, 0, b2.Depth())
}

func TestJoinPath(t *testing.T) {
	assert.Equal(t, "member 0", JoinPath("", "member 0"))
	assert.Equal(t, "member 0/ring 1", JoinPath("member 0", "ring 1"))
	assert.Equal(t, "member 0/member 2/ring 0", JoinPath("member 0/member 2", "ring 0"))
}

func TestErrorStrings(t *testing.T) {
	assert.Equal(t,
		"wkb: coordinates limit exceeded at member 1/ring 0: requested 4, used 7, limit 10",
		ErrLimitExceeded{
			What:      "coordinates",
			Path:      "member 1/ring 0",
			Requested: 4,
			Used:      7,
			Limit:     10,
		}.Error(),
	)
	assert.Equal(t,
		"wkb: depth limit exceeded: requested 3, used 2, limit 2",
		ErrLimitExceeded{
			What:      "depth",
			Requested: 3,
			Used:      2,
			Limit:     2,
		}.Error(),
	)
	assert.Equal(t,
		"wkb: integer overflow in coordinate count at ring 0: 17179869180",
		ErrIntegerOverflow{
			What:  "coordinate count",
			Path:  "ring 0",
			Value: 17179869180,
		}.Error(),
	)
}
