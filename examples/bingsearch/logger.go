package bingsearch

import stdlog "log"

// StdLogger is a minimal automationfleet.Logger over the standard library
// logger, shared so each of the four mains doesn't redefine it (see
// examples/five_browser_steps/main.go's stdLogger for the pattern this
// mirrors).
type StdLogger struct{}

func (StdLogger) Infof(f string, a ...any)  { stdlog.Printf("[info] "+f, a...) }
func (StdLogger) Warnf(f string, a ...any)  { stdlog.Printf("[warn] "+f, a...) }
func (StdLogger) Errorf(f string, a ...any) { stdlog.Printf("[err]  "+f, a...) }
func (StdLogger) Debugf(f string, a ...any) {}
