package v2

import "strings"

var authenticationAllowlistMethods = map[string]bool{
	"/memos.api.v2.SystemService/GetSystemInfo":   true,
	"/memos.api.v2.AuthService/GetAuthStatus":     true,
	"/memos.api.v2.UserService/GetUser":           true,
	"/memos.api.v2.MemoService/ListMemos":         true,
	"/memos.api.v2.MemoService/GetMemo":           true,
	"/memos.api.v2.MemoService/GetMemoByName":     true,
	"/memos.api.v2.MemoService/ListMemoResources": true,
	"/memos.api.v2.MemoService/ListMemoRelations": true,
	"/memos.api.v2.MemoService/ListMemoComments":  true,
	"/memos.api.v2.MarkdownService/ParseMarkdown": true,
}

// isUnauthorizeAllowedMethod returns whether the method is exempted from authentication.
func isUnauthorizeAllowedMethod(fullMethodName string) bool {
	if strings.HasPrefix(fullMethodName, "/grpc.reflection") {
		return true
	}
	return authenticationAllowlistMethods[fullMethodName]
}

var allowedMethodsOnlyForAdmin = map[string]bool{
	"/memos.api.v2.UserService/CreateUser": true,
}

// isOnlyForAdminAllowedMethod returns true if the method is allowed to be called only by admin.
func isOnlyForAdminAllowedMethod(methodName string) bool {
	return allowedMethodsOnlyForAdmin[methodName]
}
