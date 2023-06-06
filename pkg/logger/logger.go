package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
	"time"
)

const (
	TimeFormat = "2006/01/02 15:04:05"
)

var (
	Logger *zap.Logger
	Sugar  *zap.SugaredLogger
)

func Init() {
	writeSyncer := zapcore.NewMultiWriteSyncer(zapcore.AddSync(os.Stdout))
	encoder := getEncoder()
	core := zapcore.NewCore(encoder, writeSyncer, zapcore.DebugLevel)
	Logger = zap.New(core, zap.AddCaller())
	Sugar = Logger.Sugar()
}

func getEncoder() zapcore.Encoder {
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = encodeTime
	encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	return zapcore.NewConsoleEncoder(encoderConfig)
}

func encodeTime(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString("[" + t.Format(TimeFormat) + "]")
}
