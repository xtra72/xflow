package dto

// APIResponse 는 표준 API 응답 엔벨로프이다.
// 모든 API 응답은 반드시 이 형식으로 래핑되어야 한다.
type APIResponse[T any] struct {
	Success bool         `json:"success"`
	Data    T            `json:"data,omitempty"`
	Error   *ErrorDetail `json:"error,omitempty"`
	Meta    *Meta        `json:"meta,omitempty"`
}

// ErrorDetail 은 에러 정보를 포함한다.
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// Meta 는 응답 메타데이터를 포함한다.
type Meta struct {
	RequestID  string          `json:"request_id,omitempty"`
	Pagination *PaginationMeta `json:"pagination,omitempty"`
}

// PaginationMeta 는 목록 응답에 대한 페이지네이션 정보를 포함한다.
type PaginationMeta struct {
	Page       int   `json:"page"`
	Size       int   `json:"size"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

// NewSuccessResponse 는 데이터를 포함하는 성공 응답을 생성한다.
func NewSuccessResponse[T any](data T) APIResponse[T] {
	return APIResponse[T]{
		Success: true,
		Data:    data,
	}
}

// NewSuccessResponseWithMeta 는 데이터와 메타를 포함하는 성공 응답을 생성한다.
func NewSuccessResponseWithMeta[T any](data T, meta *Meta) APIResponse[T] {
	return APIResponse[T]{
		Success: true,
		Data:    data,
		Meta:    meta,
	}
}

// NewErrorResponse 는 에러 응답을 생성한다.
func NewErrorResponse(code, message string, details ...any) APIResponse[any] {
	errDetail := &ErrorDetail{
		Code:    code,
		Message: message,
	}
	if len(details) > 0 {
		errDetail.Details = details[0]
	}

	return APIResponse[any]{
		Success: false,
		Error:   errDetail,
	}
}

// NewPaginatedResponse 는 페이지네이션 메타를 포함하는 성공 응답을 생성한다.
func NewPaginatedResponse[T any](data T, page, size int, total int64) APIResponse[T] {
	totalPages := 0
	if size > 0 {
		totalPages = int((total + int64(size) - 1) / int64(size))
	}

	return APIResponse[T]{
		Success: true,
		Data:    data,
		Meta: &Meta{
			Pagination: &PaginationMeta{
				Page:       page,
				Size:       size,
				Total:      total,
				TotalPages: totalPages,
			},
		},
	}
}
