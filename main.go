package main

import (
	"flag"

	"github.com/YuJuncen/diskbusy/diskbusy"
	"github.com/gin-gonic/gin"
)

var (
	runningAt = flag.String("path", "", "the candidante for reading")
	addr      = flag.String("port", "0.0.0.0:8081", "the http port to listen")
)

func main() {
	flag.Parse()
	handler := diskbusy.NewHandler(*runningAt)
	app := gin.New()
	handler.Register(app)
	app.Run(*addr)
}
