package model

import "time"

type DNSEvent struct {
	Timestamp  time.Time
	ServerName string
	ServerRole string

	ClientIP     string
	QueryName    string
	QueryType    string
	ResponseCode string

	AnswerIP string
	Protocol string

	RawLine string
	RawHash string
}
