package workspace

const (
	NotFound  = Error("Not Found")
	Forbidden = Error("Forbidden")
)

type Error string

func (e Error) Error() string {
	return string(e)
}
