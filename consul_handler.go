package emissary

type consulResultHandler interface {
	handle(result consulEdsResult)
}
