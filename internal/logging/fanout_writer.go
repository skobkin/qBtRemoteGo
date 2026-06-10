package logging

import "io"

// fanoutWriter mirrors each Write to all of its underlying writers.
// It returns nil error if any writer accepted the full payload, and
// the first error otherwise. Short writes are converted to
// io.ErrShortWrite.
type fanoutWriter struct {
	writers []io.Writer
}

func newFanoutWriter(writers ...io.Writer) io.Writer {
	filtered := make([]io.Writer, 0, len(writers))
	for _, w := range writers {
		if w != nil {
			filtered = append(filtered, w)
		}
	}

	return &fanoutWriter{writers: filtered}
}

func (w *fanoutWriter) Write(p []byte) (int, error) {
	var (
		wroteAny bool
		firstErr error
	)

	for _, dst := range w.writers {
		n, err := dst.Write(p)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}

			continue
		}
		if n != len(p) {
			if firstErr == nil {
				firstErr = io.ErrShortWrite
			}

			continue
		}
		wroteAny = true
	}

	if wroteAny {
		return len(p), nil
	}
	if firstErr != nil {
		return 0, firstErr
	}

	return len(p), nil
}
