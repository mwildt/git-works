package common

type (
	Predicate[T any] func(T) bool
	Consumer[T any]  func(T) error
	Producer[T any]  func() (T, error)
)

func NoopConsumer[T any](_ T) error {
	return nil
}
