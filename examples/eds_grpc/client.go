package main

import (
	"github.com/apex/log"
	"io/ioutil"
	"net/http"
	"time"
)

func main() {
	for {
		resp, err := http.DefaultClient.Get("http://client-envoy")
		if err != nil {
			log.Info(err.Error())
		} else {
			buf, _ := ioutil.ReadAll(resp.Body)
			log.Infof("resp is %s", string(buf))
		}

		time.Sleep(1 * time.Second)
	}
}
