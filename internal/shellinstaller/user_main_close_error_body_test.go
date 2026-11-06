package shellinstaller

import "io"

type userMainCloseErrorBody struct {
	io.Reader
	cause  error
	closed *int
}

func (b userMainCloseErrorBody) Close() error {
	(*b.closed)++
	return b.cause
}
