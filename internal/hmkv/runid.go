package hmkv

import (
	"fmt"
	"os"
	"time"
)

// newRunOutputSlug returns a unique per-process output directory name so overlapping
// handymkv runs never share the same mkv/hb staging folders.
func newRunOutputSlug(start time.Time) string {
	return fmt.Sprintf("handymkv_%s_%d_%d", start.Format("2006-01-02_15-04-05"), os.Getpid(), start.UnixNano())
}

// manifestFileName returns a unique manifest JSON filename for this run.
func manifestFileName(start time.Time) string {
	return fmt.Sprintf("manifest_%s_%d_%d.json", start.Format("2006-01-02_15-04-05"), os.Getpid(), start.UnixNano())
}
