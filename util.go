package timewheel

import "time"

func CalcCurTicketMS(ticketMS int64, now ...time.Time) int64 {
	var nowMS int64
	if len(now) > 0 {
		nowMS = now[0].UnixMilli()
	} else {
		nowMS = time.Now().UnixMilli()
	}
	return nowMS - nowMS%ticketMS
}
