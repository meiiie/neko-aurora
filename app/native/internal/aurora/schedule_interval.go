package aurora

import (
	"strconv"
	"strings"
	"time"
)

func validScheduleInterval(minutes int) bool { return minutes >= 15 && minutes <= 10080 }

func (x schedule) intervalPrefix() string {
	return "i" + strconv.FormatUint(x.IntervalGeneration, 10) + "-"
}

func (x schedule) intervalState() (time.Time, int64, bool) {
	anchor, err := time.Parse(time.RFC3339Nano, x.IntervalAnchor)
	if err != nil || anchor.UTC().Format(time.RFC3339Nano) != x.IntervalAnchor || anchor.Year() < 1970 || !validScheduleInterval(x.IntervalMinutes) || x.IntervalGeneration == 0 || x.IntervalGeneration > max(1, x.Revision) || x.IntervalGeneration >= 1<<53 || x.At != "" || x.Date != "" || x.StartDate != "" {
		return time.Time{}, 0, false
	}
	var last int64
	if x.Last != "" {
		prefix := x.intervalPrefix()
		if !strings.HasPrefix(x.Last, prefix) {
			return time.Time{}, 0, false
		}
		slot := strings.TrimPrefix(x.Last, prefix)
		last, err = strconv.ParseInt(slot, 10, 53)
		if err != nil || last < 1 || strconv.FormatInt(last, 10) != slot {
			return time.Time{}, 0, false
		}
	}
	return anchor, last, true
}

func (x schedule) intervalDue(now time.Time) string {
	anchor, last, ok := x.intervalState()
	if !ok || now.Before(anchor) {
		return ""
	}
	seconds := now.Unix() - anchor.Unix()
	if now.Nanosecond() < anchor.Nanosecond() {
		seconds--
	}
	slot := seconds / (int64(x.IntervalMinutes) * 60)
	if slot < 1 || slot <= last {
		return ""
	}
	return x.intervalPrefix() + strconv.FormatInt(slot, 10)
}

func (x *schedule) startInterval(now time.Time, generation uint64) {
	x.At, x.Date, x.StartDate, x.Last = "", "", "", ""
	x.IntervalAnchor = now.UTC().Format(time.RFC3339Nano)
	x.IntervalGeneration = generation
}

func (x *schedule) clearInterval() {
	x.IntervalMinutes, x.IntervalAnchor, x.IntervalGeneration = 0, "", 0
}
