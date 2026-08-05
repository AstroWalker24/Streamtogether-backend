package api

import "time"


type Meta struct {
    Timestamp  time.Time   `json:"timestamp"`
    RequestID  string      `json:"request_id,omitempty"`
    Version    string      `json:"version,omitempty"`
    Pagination *Pagination `json:"pagination,omitempty"`
}


func NewMeta() *Meta {
    return &Meta{Timestamp: time.Now().UTC()}
}


func (m *Meta) WithRequestID(id string) *Meta {
    m.RequestID = id
    return m
}


func (m *Meta) WithPagination(p *Pagination) *Meta {
    m.Pagination = p
    return m
}


func (m *Meta) WithVersion(v string) *Meta {
    m.Version = v
    return m
}