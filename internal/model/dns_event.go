package model

import "time"

type DNSEvent struct {
	Timestamp  time.Time
	ServerName string
	ServerRole string

	ClientIP     string
	QueryName    string
	QueryClass   string
	QueryType    string
	ResponseCode string

	AnswerIP       string
	Protocol       string
	SourceCategory string

	RawLine string
	RawHash string
}
