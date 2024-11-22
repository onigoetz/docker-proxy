package lib

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	write "github.com/influxdata/influxdb-client-go/v2/api/write"
)

type InfluxConfig struct {
	Url    string
	Token  string
	Org    string
	Bucket string
}

func writePoint(config InfluxConfig, point *write.Point) {

	// Create a new client using an InfluxDB server base URL and an authentication token
	client := influxdb2.NewClient(config.Url, config.Token)

	defer client.Close()

	// Use blocking write client for writes to desired bucket
	writeAPI := client.WriteAPIBlocking(config.Org, config.Bucket)

	err := writeAPI.WritePoint(context.Background(), point)
	if err != nil {
		slog.Error(fmt.Sprintf("Failed to write point: %v", err))
	}
}

func NewContainerCreation(config InfluxConfig, image string) {
	log.Printf("Container creation: %s\n", image)

	p := influxdb2.NewPointWithMeasurement("docker_container_create").
		AddTag("image", image).
		AddField("count", 1).
		SetTime(time.Now())

	writePoint(config, p)
}

func ImagePull(config InfluxConfig, image string) {
	log.Printf("Image pull: %s\n", image)

	p := influxdb2.NewPointWithMeasurement("docker_image_pull").
		AddTag("image", image).
		AddField("count", 1).
		SetTime(time.Now())

	writePoint(config, p)
}
