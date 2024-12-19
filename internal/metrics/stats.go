package metrics

import "time"

func (r *MetricsRecorder) runUpdateStats(updateChan <-chan []byte) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	// var msg []byte
	for {
		select {
		case _, ok := <-updateChan:
			if !ok {
				return
			}
		case <-ticker.C:
			if msg, ok := <-updateChan; ok {
				r.logger.WithField("book_update", string(msg)).Info("Received update")
			} else {
				return
			}
		case <-r.Done():
			return
		}
	}
}
