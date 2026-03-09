package main

import (
	"fmt"
	"log"
	"time"

	"github.com/s-anzie/gx/core"
)

func main() {
	app := core.New()

	app.GET("/events", eventsHandler)
	app.GET("/counter", counterHandler)
	app.GET("/", indexHandler)

	log.Println("Server starting on :8080")
	log.Fatal(app.Listen("localhost", "8080"))
}

func eventsHandler(c *core.Context) core.Response {
	return core.SSE(func(stream *core.SSEStream) error {
		stream.Send(core.SSEEvent{
			Event: "connected",
			Data:  "Connection established",
		})

		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		go core.SSEKeepAlive(stream, 30*time.Second)

		for {
			select {
			case <-stream.Context().Done():
				return nil
			case t := <-ticker.C:
				if err := stream.Send(core.SSEEvent{
					Event: "time",
					Data:  t.Format(time.RFC3339),
				}); err != nil {
					return err
				}
			}
		}
	})
}

func counterHandler(c *core.Context) core.Response {
	return core.SSE(func(stream *core.SSEStream) error {
		counter := 0
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-stream.Context().Done():
				return nil
			case <-ticker.C:
				counter++
				if err := stream.Send(core.SSEEvent{
					ID:    fmt.Sprintf("%d", counter),
					Event: "count",
					Data:  fmt.Sprintf(`{"count": %d}`, counter),
				}); err != nil {
					return err
				}

				if counter >= 20 {
					stream.Send(core.SSEEvent{
						Event: "done",
						Data:  "Counter finished",
					})
					return nil
				}
			}
		}
	})
}

func indexHandler(c *core.Context) core.Response {
	htmlContent := `<!DOCTYPE html>
<html>
<head>
    <title>GX SSE Demo</title>
</head>
<body>
    <h1>GX Server-Sent Events Demo</h1>
    <h2>Time Updates</h2>
    <div id="time-events"></div>
    <h2>Counter</h2>
    <div id="counter-events"></div>
    <script>
        var timeSource = new EventSource('/events');
        timeSource.addEventListener('time', function(e) {
            document.getElementById('time-events').innerHTML = e.data;
        });
        var counterSource = new EventSource('/counter');
        counterSource.addEventListener('count', function(e) {
            document.getElementById('counter-events').innerHTML = e.data;
        });
    </script>
</body>
</html>`

	return c.HTML(htmlContent)
}
