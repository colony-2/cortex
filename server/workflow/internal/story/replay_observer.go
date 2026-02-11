package story

import "github.com/colony-2/swf-go/pkg/swf"

type replayStoryObserver struct {
	rec *replayStoryRecorder
}

func (o *replayStoryObserver) OnJobStart(event swf.JobStartEvent) {
	if o == nil || o.rec == nil {
		return
	}
	o.rec.OnJobStartAttempt(event.AttemptNumber)
}

func (o *replayStoryObserver) OnTaskStart(event swf.TaskStartEvent) {
	if o == nil || o.rec == nil {
		return
	}
	o.rec.OnTaskStart(event)
}

func (o *replayStoryObserver) OnTaskEnd(event swf.TaskEndEvent) {
	if o == nil || o.rec == nil {
		return
	}
	o.rec.OnTaskEnd(event)
}

func (o *replayStoryObserver) OnJobEnd(event swf.JobEndEvent) {
	// No-op for now; story nodes are finalized by the decorated executor + task events.
	_ = event
}

var _ swf.ReplayObserver = (*replayStoryObserver)(nil)
