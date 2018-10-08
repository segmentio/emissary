package emissary

import "sync"

type consulResultHandler interface {
	handle(wg *sync.WaitGroup, result consulEdsResult)
}
