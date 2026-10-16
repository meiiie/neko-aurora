package runtime

import "io"

type hostFrameWriter struct{ io.Writer }

func (w hostFrameWriter) Write(data []byte) (int, error) {
	total := 0
	for len(data) > 0 {
		next := len(data)
		if next > 1024 {
			next = 1024
		}
		n, err := w.Writer.Write(data[:next])
		total += n
		if err != nil {
			return total, err
		}
		if n != next {
			return total, io.ErrShortWrite
		}
		data = data[n:]
	}
	return total, nil
}
