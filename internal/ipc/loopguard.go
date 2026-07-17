// loopguard.go owns the stream-side runaway-loop detector used by handleStream
// (stream.go) to cancel generation when a model gets stuck emitting the same
// short sequence over and over. it does NOT own the stream loop itself.

package ipc

import "bytes"

// hasRepeatedTail reports whether s ends with a short sequence repeated at least
// three times in a row (e.g. "for_for_for"), the signature of a model stuck in a
// generation loop. only the recent tail is inspected so the check stays cheap when
// run per streamed chunk. the minimum period is 3 and the repeating block must
// contain a letter or digit, so runs of punctuation/whitespace ("-----") or a long
// zero-padded number ("3.000000") do not trip it.
func hasRepeatedTail(s string) bool {
	const (
		window     = 128 // inspect only the recent tail; a live loop shows up here
		maxPeriod  = 40
		minPeriod  = 3
		minRepeats = 3
	)
	// slice the string tail BEFORE converting to []byte: only the last `window`
	// bytes are inspected, so converting the whole (possibly huge) reply-so-far
	// would be an O(len) copy on every streamed chunk. slicing a string is a
	// header-only op, so the []byte copy is bounded to `window`.
	if len(s) > window {
		s = s[len(s)-window:]
	}
	b := []byte(s)
	n := len(b)
	for p := minPeriod; p <= maxPeriod; p++ {
		if p*minRepeats > n {
			break
		}
		block := b[n-p:]
		if !hasAlnum(block) {
			continue
		}
		// count how many consecutive copies of block end the buffer.
		repeats := 1
		for k := 2; k*p <= n; k++ {
			if !bytes.Equal(b[n-k*p:n-(k-1)*p], block) {
				break
			}
			repeats++
		}
		if repeats >= minRepeats {
			return true
		}
	}
	return false
}

// hasAlnum reports whether b contains at least one ascii letter or digit.
func hasAlnum(b []byte) bool {
	for _, c := range b {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			return true
		}
	}
	return false
}
