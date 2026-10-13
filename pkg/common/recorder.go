package common

type Recorder[T any] []T

func (c *Recorder[T]) Record(item T) {
	*c = append(*c, item)
}

func (c *Recorder[T]) RecordErr(item T) error {
	*c = append(*c, item)
	return nil
}
