package tui

// logStream owns the client-side offset bookkeeping for the detail view's
// incremental log-fetch protocol (client.GetLogs(taskID, offset)). Before
// this type existed, that bookkeeping was smeared across three places: the
// tick loop read m.detail.contentLineCount to compute the next offset, the
// outputLoadedMsg reducer branched on msg.offset>0 to pick AppendNewLines
// vs SetOutput, and detailView carried contentLineCount partly for this wire
// purpose. logStream concentrates it: it tracks which task is currently
// streamed and how many lines have been fetched so far, so callers ask for
// "the next page" instead of reaching into the view's internals.
//
// The wire protocol itself (GetLogs(taskID, offset) — offset 0 means "give
// me everything", callers query at the current output line count and threads
// totalLines to the next offset) is unchanged; this is purely client-side
// locality.
type logStream struct {
	taskID int64
	offset int
}

// reset points the stream at a new task, forcing the next request to be a
// full load (offset 0). Call this whenever the detail view switches tasks.
func (ls *logStream) reset(taskID int64) {
	ls.taskID = taskID
	ls.offset = 0
}

// nextRequest returns the (taskID, offset) pair to pass to loadOutput for
// the next fetch.
func (ls *logStream) nextRequest() (int64, int) {
	return ls.taskID, ls.offset
}

// apply reduces a loaded output page into the detail view: offset 0 was a
// full load (SetOutput), anything else was an incremental fetch
// (AppendNewLines). It advances the tracked offset to the server-reported
// total line count so the next nextRequest() picks up where this one left
// off. Returns false and leaves the detail view untouched if msg belongs to
// a task that's no longer being streamed (a stale response racing a task
// switch).
func (ls *logStream) apply(msg outputLoadedMsg, d *detailView) bool {
	if msg.taskID != ls.taskID {
		return false
	}
	if msg.offset > 0 {
		d.AppendNewLines(msg.lines)
	} else {
		d.SetOutput(msg.lines)
	}
	ls.offset = msg.totalLines
	return true
}
