package utils

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
)

func SleepContext(ctx context.Context, d time.Duration) {
	logrus.Debugf("Sleeping for %s...", d.String())

	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}
