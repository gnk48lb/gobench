package logger

import (
	"go.uber.org/zap"
)

var Log *zap.Logger

func Init() error {
	var err error
	Log, err = zap.NewProduction()
	if err != nil {
		return err
	}
	return nil
}

func Sync() {
	if Log != nil {
		_ = Log.Sync()
	}
}
