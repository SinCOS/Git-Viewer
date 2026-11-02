//go:build !unix

package remotehelper

func interruptibleStream(stream any) (any, error) { return stream, nil }
