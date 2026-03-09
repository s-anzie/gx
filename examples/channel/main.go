package main

import (
	"fmt"
	"log"
	"time"

	"github.com/s-anzie/gx"
	"github.com/s-anzie/gx/core"
)

type Notification struct {
	Type    string
	Message string
	Time    time.Time
}

func main() {
	app := gx.New(
		gx.WithEnvironment(gx.Development),
		gx.WithStructuredLogs(),
	)

	app.GET("/stream/notifications", gx.WrapHandlerWithApp(app, notificationHandler))
	app.GET("/stream/time", gx.WrapHandlerWithApp(app, timeHandler))

	fmt.Println("Channel example on :8082")
	fmt.Println("Try: curl http://localhost:8082/stream/time")

	if err := app.Listen(":8080"); err != nil {
		log.Fatal(err)
	}
}

func notificationHandler(c *gx.Context) core.Response {
	ch, err := c.SSEChannel()
	if err != nil {
		return c.Fail(gx.ErrInternal.With("msg", "channel error"))
	}
	defer ch.Close()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	count := 0
	for {
		select {
		case <-ch.Done():
			return nil
		case <-ticker.C:
			count++
			notif := Notification{
				Type:    "update",
				Message: fmt.Sprintf("Notification #%d", count),
				Time:    time.Now(),
			}
			ch.Send(notif)
		}
	}
}

func timeHandler(c *gx.Context) core.Response {
	ch, err := c.SSEChannel()
	if err != nil {
		return c.Fail(gx.ErrInternal.With("msg", "channel error"))
	}
	defer ch.Close()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ch.Done():
			return nil
		case t := <-ticker.C:
			data := map[string]any{
				"time": t.Format(time.RFC3339),
				"unix": t.Unix(),
			}
			ch.Send(data)
		}
	}
}
