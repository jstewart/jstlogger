package callbook_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"jstlogger/internal/callbook"
	"jstlogger/internal/callbook/mock"
)

func TestChainEmpty(t *testing.T) {
	_, err := callbook.Chain{}.Lookup(t.Context(), "W1AW")
	assert.ErrorIs(t, err, callbook.ErrNotFound)
}

func TestChainFirstSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	first := mock.NewMockLooker(ctrl)
	second := mock.NewMockLooker(ctrl)
	// third must not be called.
	third := mock.NewMockLooker(ctrl)

	want := callbook.Result{Call: "W1AW", Name: "Hiram", Service: callbook.ServiceHamQTH}
	gomock.InOrder(
		first.EXPECT().Lookup(gomock.Any(), "W1AW").Return(callbook.Result{}, &callbook.NetworkError{Service: callbook.ServiceQRZ, Err: errors.New("down")}),
		second.EXPECT().Lookup(gomock.Any(), "W1AW").Return(want, nil),
	)

	got, err := callbook.Chain{first, second, third}.Lookup(t.Context(), "W1AW")
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestChainAllNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	a := mock.NewMockLooker(ctrl)
	b := mock.NewMockLooker(ctrl)
	a.EXPECT().Lookup(gomock.Any(), "N0CALL").Return(callbook.Result{}, callbook.ErrNotFound)
	b.EXPECT().Lookup(gomock.Any(), "N0CALL").Return(callbook.Result{}, fmt.Errorf("hamqth: %w", callbook.ErrNotFound))

	_, err := callbook.Chain{a, b}.Lookup(t.Context(), "N0CALL")
	assert.ErrorIs(t, err, callbook.ErrNotFound)
}

func TestChainJoinsErrors(t *testing.T) {
	ctrl := gomock.NewController(t)
	a := mock.NewMockLooker(ctrl)
	b := mock.NewMockLooker(ctrl)
	c := mock.NewMockLooker(ctrl)
	netErr := &callbook.NetworkError{Service: callbook.ServiceHamQTH, Err: errors.New("timeout")}
	a.EXPECT().Lookup(gomock.Any(), "W1AW").Return(callbook.Result{}, callbook.ErrAuth)
	b.EXPECT().Lookup(gomock.Any(), "W1AW").Return(callbook.Result{}, netErr)
	c.EXPECT().Lookup(gomock.Any(), "W1AW").Return(callbook.Result{}, callbook.ErrNotFound)

	_, err := callbook.Chain{a, b, c}.Lookup(t.Context(), "W1AW")
	require.Error(t, err)
	assert.ErrorIs(t, err, callbook.ErrAuth)
	assert.ErrorIs(t, err, callbook.ErrNotFound)
	var ne *callbook.NetworkError
	require.ErrorAs(t, err, &ne)
	assert.Equal(t, callbook.ServiceHamQTH, ne.Service)
}

func TestChainCanceled(t *testing.T) {
	ctrl := gomock.NewController(t)
	a := mock.NewMockLooker(ctrl)
	b := mock.NewMockLooker(ctrl) // must not be called

	ctx, cancel := context.WithCancel(t.Context())
	a.EXPECT().Lookup(gomock.Any(), "W1AW").DoAndReturn(func(context.Context, string) (callbook.Result, error) {
		cancel()
		return callbook.Result{}, callbook.ErrNotFound
	})

	_, err := callbook.Chain{a, b}.Lookup(ctx, "W1AW")
	assert.ErrorIs(t, err, context.Canceled)
}

func TestChainCanceledBeforeStart(t *testing.T) {
	ctrl := gomock.NewController(t)
	a := mock.NewMockLooker(ctrl) // must not be called

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := callbook.Chain{a}.Lookup(ctx, "W1AW")
	assert.ErrorIs(t, err, context.Canceled)
}
