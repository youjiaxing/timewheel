package timewheel

import "time"

func CalcCurTicketMS(ticketMS int64) int64 {
	nowMS := time.Now().UnixMilli()
	return nowMS - nowMS%ticketMS
}
