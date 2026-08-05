package errors

import "net/http"


var statusMap = map[Code]int{
    CodeInternalServerError: http.StatusInternalServerError,
    CodeValidationError:     http.StatusBadRequest,
    CodeBadRequest:          http.StatusBadRequest,
    CodeUnauthorized:        http.StatusUnauthorized,
    CodeForbidden:           http.StatusForbidden,
    CodeNotFound:            http.StatusNotFound,
    CodeConflict:            http.StatusConflict,
    CodeDatabaseError:       http.StatusInternalServerError,
    CodeRedisError:          http.StatusInternalServerError,
    CodeTimeout:             http.StatusGatewayTimeout,
    CodeUnknownError:        http.StatusInternalServerError,
}


func HTTPStatus(code Code) int {
    if s, ok := statusMap[code]; ok {
        return s
    }
    return http.StatusInternalServerError
}