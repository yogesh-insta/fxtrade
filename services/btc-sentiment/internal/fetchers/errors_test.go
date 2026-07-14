package fetchers_test

import (
	"errors"
	"testing"

	"github.com/ym/fxtrade/services/btc-sentiment/internal/fetchers"
)

func TestFetchErrorSentinels(t *testing.T) {
	noData := fetchers.NewNoDataError("FetchNews")
	if !errors.Is(noData, fetchers.ErrNoData) {
		t.Fatal("expected ErrNoData")
	}

	fail := fetchers.NewFetchError("FetchNews", errors.New("boom"))
	if errors.Is(fail, fetchers.ErrNoData) {
		t.Fatal("request failure should not be ErrNoData")
	}
	if fail.Error() == "" {
		t.Fatal("empty error string")
	}
}
