package exception

import (
	"encoding/json"
	"net/http"
)

type ErrorCode struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Error   string `json:"error"`
	Status  int    `json:"-"`
}

var (
	// Account errors (1000-1999)
	AccountNotFound = ErrorCode{
		Code:    1000,
		Message: "Account not found",
		Error:   "ACCOUNT_NOT_FOUND",
		Status:  http.StatusNotFound,
	}
	AccountNotVerified = ErrorCode{
		Code:    1001,
		Message: "Account not verified",
		Error:   "ACCOUNT_NOT_VERIZED",
		Status:  http.StatusUnauthorized,
	}
	AccountLocked = ErrorCode{
		Code:    1002,
		Message: "Account locked",
		Error:   "ACCOUNT_LOCKED",
		Status:  http.StatusLocked,
	}
	AuthenticationFailed = ErrorCode{
		Code:    1003,
		Message: "Authentication failed",
		Error:   "AUTHENTICATION_FAILED",
		Status:  http.StatusUnauthorized,
	}
	InvalidPassword = ErrorCode{
		Code:    1004,
		Message: "Invalid password",
		Error:   "INVALID_PASSWORD",
		Status:  http.StatusBadRequest,
	}
	InvalidEmail = ErrorCode{
		Code:    1005,
		Message: "Invalid email",
		Error:   "INVALID_EMAIL",
		Status:  http.StatusBadRequest,
	}
	EmailRequired = ErrorCode{
		Code:    1006,
		Message: "Email is required",
		Error:   "EMAIL_REQUIRED",
		Status:  http.StatusBadRequest,
	}
	PasswordRequired = ErrorCode{
		Code:    1007,
		Message: "Password is required",
		Error:   "PASSWORD_REQUIRED",
		Status:  http.StatusBadRequest,
	}
	VerificationCodeNotFound = ErrorCode{
		Code:    1008,
		Message: "Verification code not found",
		Error:   "VERIFICATION_CODE_NOT_FOUND",
		Status:  http.StatusNotFound,
	}
	VerificationCodeNotMatchedOrExpired = ErrorCode{
		Code:    1009,
		Message: "Verification code not matched or expired",
		Error:   "VERIFICATION_CODE_NOT_MATCHED_OR_EXPIRED",
		Status:  http.StatusBadRequest,
	}
	RefreshTokenRequired = ErrorCode{
		Code:    1010,
		Message: "Refresh token is required",
		Error:   "REFRESH_TOKEN_REQUIRED",
		Status:  http.StatusBadRequest,
	}
	InvalidOrExpiredRefreshToken = ErrorCode{
		Code:    1011,
		Message: "Invalid or expired refresh token",
		Error:   "INVALID_OR_EXPIRED_REFRESH_TOKEN",
		Status:  http.StatusBadRequest,
	}
	AccountAlreadyExists = ErrorCode{
		Code:    1012,
		Message: "Account already exists",
		Error:   "ACCOUNT_ALREADY_EXISTS",
		Status:  http.StatusConflict,
	}
	InvalidToken = ErrorCode{
		Code:    1013,
		Message: "Invalid token",
		Error:   "INVALID_TOKEN",
		Status:  http.StatusBadRequest,
	}
	ExpiredToken = ErrorCode{
		Code:    1014,
		Message: "Expired token",
		Error:   "EXPIRED_TOKEN",
		Status:  http.StatusBadRequest,
	}
	VerificationCodeRequired = ErrorCode{
		Code:    1015,
		Message: "Verification code is required",
		Error:   "VERIFICATION_CODE_REQUIRED",
		Status:  http.StatusBadRequest,
	}
	AccountVerified = ErrorCode{
		Code:    1016,
		Message: "Account verified",
		Error:   "ACCOUNT_VERIFIED",
		Status:  http.StatusConflict,
	}

	// User errors (2000-2999)
	GivenNameRequired = ErrorCode{
		Code:    2000,
		Message: "Given name is required",
		Error:   "GIVEN_NAME_REQUIRED",
		Status:  http.StatusBadRequest,
	}
	FamilyNameRequired = ErrorCode{
		Code:    2001,
		Message: "Family name is required",
		Error:   "FAMILY_NAME_REQUIRED",
		Status:  http.StatusBadRequest,
	}
	BirthdateRequired = ErrorCode{
		Code:    2002,
		Message: "Birth date is required",
		Error:   "BIRTHDATE_REQUIRED",
		Status:  http.StatusBadRequest,
	}
	AgeMustBeAtLeast16 = ErrorCode{
		Code:    2003,
		Message: "This user is under age",
		Error:   "AGE_MUST_BE_AT_LEAST_16",
		Status:  http.StatusBadRequest,
	}
	InvalidGivenNameLength = ErrorCode{
		Code:    2004,
		Message: "Given name is too long",
		Error:   "INVALID_GIVEN_NAME_LENGTH",
		Status:  http.StatusBadRequest,
	}
	InvalidFamilyNameLength = ErrorCode{
		Code:    2005,
		Message: "Family name is too long",
		Error:   "INVALID_FAMILY_NAME_LENGTH",
		Status:  http.StatusBadRequest,
	}
	LessThan30DaysSinceLastBirthdateChange = ErrorCode{
		Code:    2006,
		Message: "Less than 30 days since last date of birth change",
		Error:   "LESS_THAN_30_DAYS_SINCE_LAST_BIRTHDATE_CHANGE",
		Status:  http.StatusBadRequest,
	}
	LessThan30DaysSinceLastNameChange = ErrorCode{
		Code:    2007,
		Message: "Less than 30 days since last name change",
		Error:   "LESS_THAN_30_DAYS_SINCE_LAST_NAME_CHANGE",
		Status:  http.StatusBadRequest,
	}
	LessThan30DaysSinceLastUsernameChange = ErrorCode{
		Code:    2008,
		Message: "Less than 30 days since last username change",
		Error:   "LESS_THAN_30_DAYS_SINCE_LAST_USERNAME_CHANGE",
		Status:  http.StatusBadRequest,
	}
	EmailNotVerified = ErrorCode{
		Code:    2009,
		Message: "Email is not verified",
		Error:   "EMAIL_NOT_VERIFIED",
		Status:  http.StatusUnauthorized,
	}
	UserNotFound = ErrorCode{
		Code:    2010,
		Message: "User not found",
		Error:   "USER_NOT_FOUND",
		Status:  http.StatusNotFound,
	}
	InvalidUsername = ErrorCode{
		Code:    2010,
		Message: "Invalid username",
		Error:   "INVALID_USERNAME",
		Status:  http.StatusBadRequest,
	}
	UsernameRequired = ErrorCode{
		Code:    2011,
		Message: "Username is required",
		Error:   "USERNAME_REQUIRED",
		Status:  http.StatusBadRequest,
	}
	UsernameAlreadyExists = ErrorCode{
		Code:    2012,
		Message: "Username already exists",
		Error:   "USERNAME_ALREADY_EXISTS",
		Status:  http.StatusBadRequest,
	}
	ProfilePictureRequired = ErrorCode{
		Code:    2013,
		Message: "Profile picture is required",
		Error:   "PROFILE_PICTURE_REQUIRED",
		Status:  http.StatusBadRequest,
	}
	NothingChanged = ErrorCode{
		Code:    2014,
		Message: "Nothing changed",
		Error:   "NOTHING_CHANGED",
		Status:  http.StatusBadRequest,
	}

	// Storage errors (3000-3999)
	StorageInitializationError = ErrorCode{
		Code:    3000,
		Message: "Storage initialization error",
		Error:   "STORAGE_INITIALIZATION_ERROR",
		Status:  http.StatusInternalServerError,
	}
	FileRequired = ErrorCode{
		Code:    3001,
		Message: "File is required",
		Error:   "FILE_REQUIRED",
		Status:  http.StatusBadRequest,
	}
	InvalidFile = ErrorCode{
		Code:    3002,
		Message: "Invalid file",
		Error:   "INVALID_FILE",
		Status:  http.StatusBadRequest,
	}
	UploadFileFailed = ErrorCode{
		Code:    3003,
		Message: "Upload file failed",
		Error:   "UPLOAD_FILE_FAILED",
		Status:  http.StatusInternalServerError,
	}
	FileNotFound = ErrorCode{
		Code:    3004,
		Message: "File not found",
		Error:   "FILE_NOT_FOUND",
		Status:  http.StatusNotFound,
	}
	DeleteFileFailed = ErrorCode{
		Code:    3005,
		Message: "Delete file failed",
		Error:   "DELETE_FILE_FAILED",
		Status:  http.StatusInternalServerError,
	}
	LoadFileFailed = ErrorCode{
		Code:    3006,
		Message: "Load file failed",
		Error:   "LOAD_FILE_FAILED",
		Status:  http.StatusInternalServerError,
	}
	RequiredImageFile = ErrorCode{
		Code:    3007,
		Message: "Required image file",
		Error:   "REQUIRED_IMAGE_FILE",
		Status:  http.StatusBadRequest,
	}
	InvalidFileSize = ErrorCode{
		Code:    3008,
		Message: "Invalid file size",
		Error:   "INVALID_FILE_SIZE",
		Status:  http.StatusBadRequest,
	}
	ListContainsInvalidFile = ErrorCode{
		Code:    3009,
		Message: "List contains invalid file",
		Error:   "LIST_CONTAINS_INVALID_FILE",
		Status:  http.StatusBadRequest,
	}
	RequiredImageOrVideoFile = ErrorCode{
		Code:    3010,
		Message: "Required image or video file",
		Error:   "REQUIRED_IMAGE_OR_VIDEO_FILE",
		Status:  http.StatusBadRequest,
	}

	// Friend/Block errors (4000-4999)
	CanNotMakeSelfRequest = ErrorCode{
		Code:    4000,
		Message: "Can't make self request",
		Error:   "CAN_NOT_MAKE_SELF_REQUEST",
		Status:  http.StatusBadRequest,
	}
	SentAddFriendRequestFailed = ErrorCode{
		Code:    4001,
		Message: "Sent add friend request failed",
		Error:   "SENT_ADD_FRIEND_REQUEST_FAILED",
		Status:  http.StatusBadRequest,
	}
	AddFriendRequestSentLimitReached = ErrorCode{
		Code:    4002,
		Message: "Add friend request sent limit reached",
		Error:   "ADD_FRIEND_REQUEST_SENT_LIMIT_REACHED",
		Status:  http.StatusBadRequest,
	}
	AddFriendRequestReceivedLimitReached = ErrorCode{
		Code:    4003,
		Message: "Add friend request received limit reached",
		Error:   "ADD_FRIEND_REQUEST_RECEIVED_LIMIT_REACHED",
		Status:  http.StatusBadRequest,
	}
	RequestNotFound = ErrorCode{
		Code:    4004,
		Message: "Request not found",
		Error:   "REQUEST_NOT_FOUND",
		Status:  http.StatusNotFound,
	}
	AcceptRequestFailed = ErrorCode{
		Code:    4005,
		Message: "Accept request failed",
		Error:   "ACCEPT_REQUEST_FAILED",
		Status:  http.StatusBadRequest,
	}
	HasBlocked = ErrorCode{
		Code:    4006,
		Message: "Blocked",
		Error:   "HAS_BLOCKED",
		Status:  http.StatusBadRequest,
	}
	HasBeenBlocked = ErrorCode{
		Code:    4007,
		Message: "Has been blocked",
		Error:   "HAS_BEEN_BLOCKED",
		Status:  http.StatusBadRequest,
	}
	NotBlock = ErrorCode{
		Code:    4008,
		Message: "Not blocked",
		Error:   "NOT_BLOCK",
		Status:  http.StatusBadRequest,
	}
	BlockLimitReached = ErrorCode{
		Code:    4009,
		Message: "Block limit reached",
		Error:   "BLOCK_LIMIT_REACHED",
		Status:  http.StatusBadRequest,
	}
	CanNotBlockYourself = ErrorCode{
		Code:    4010,
		Message: "Can't block yourself",
		Error:   "CAN_NOT_BLOCK_YOURSELF",
		Status:  http.StatusBadRequest,
	}
	BlockNotFound = ErrorCode{
		Code:    4011,
		Message: "Block not found",
		Error:   "BLOCK_NOT_FOUND",
		Status:  http.StatusBadRequest,
	}
	FriendNotFound = ErrorCode{
		Code:    4012,
		Message: "Friend not found",
		Error:   "FRIEND_NOT_FOUND",
		Status:  http.StatusBadRequest,
	}

	// Post errors (5000-5999)
	PostContentAndAttachFilesBothEmpty = ErrorCode{
		Code:    5000,
		Message: "Post content and attach files cannot be empty",
		Error:   "POST_CONTENT_AND_ATTACH_FILES_BOTH_EMPTY",
		Status:  http.StatusBadRequest,
	}
	InvalidPostContentLength = ErrorCode{
		Code:    5001,
		Message: "Invalid post content length",
		Error:   "INVALID_POST_CONTENT_LENGTH",
		Status:  http.StatusBadRequest,
	}
	InvalidNumberOfPostAttachments = ErrorCode{
		Code:    5002,
		Message: "Invalid number of post attachments",
		Error:   "INVALID_NUMBER_OF_POST_ATTACHMENTS",
		Status:  http.StatusBadRequest,
	}
	PostNotFound = ErrorCode{
		Code:    5003,
		Message: "Post not found",
		Error:   "POST_NOT_FOUND",
		Status:  http.StatusBadRequest,
	}
	OnlyPublicPostCanBeShared = ErrorCode{
		Code:    5005,
		Message: "Only public post can be shared",
		Error:   "ONLY_PUBLIC_POST_CAN_BE_SHARED",
		Status:  http.StatusBadRequest,
	}
	PrivacyUnchanged = ErrorCode{
		Code:    5006,
		Message: "Privacy unchanged",
		Error:   "PRIVACY_UNCHANGED",
		Status:  http.StatusBadRequest,
	}
	InvalidDeleteAttachment = ErrorCode{
		Code:    5007,
		Message: "Invalid delete attachment",
		Error:   "INVALID_DELETE_ATTACHMENT",
		Status:  http.StatusBadRequest,
	}
	PostContentUnchanged = ErrorCode{
		Code:    5008,
		Message: "Post content unchanged",
		Error:   "POST_CONTENT_UNCHANGED",
		Status:  http.StatusBadRequest,
	}
	LikedPost = ErrorCode{
		Code:    5009,
		Message: "Liked post",
		Error:   "LIKED_POST",
		Status:  http.StatusBadRequest,
	}
	NotLikedPost = ErrorCode{
		Code:    5010,
		Message: "Not liked post",
		Error:   "NOT_LIKED_POST",
		Status:  http.StatusBadRequest,
	}
	DeletedPost = ErrorCode{
		Code:    5011,
		Message: "Deleted post",
		Error:   "DELETED_POST",
		Status:  http.StatusBadRequest,
	}

	// Comment errors (6000-6999)
	CommentNotFound = ErrorCode{
		Code:    6000,
		Message: "Comment not found",
		Error:   "COMMENT_NOT_FOUND",
		Status:  http.StatusNotFound,
	}
	CommentContentAndAttachFileBothEmpty = ErrorCode{
		Code:    6001,
		Message: "Comment content and attach file cannot be empty",
		Error:   "COMMENT_CONTENT_AND_ATTACH_FILE_BOTH_EMPTY",
		Status:  http.StatusBadRequest,
	}
	InvalidCommentContentLength = ErrorCode{
		Code:    6002,
		Message: "Invalid comment content length",
		Error:   "INVALID_COMMENT_CONTENT_LENGTH",
		Status:  http.StatusBadRequest,
	}
	PostIdRequired = ErrorCode{
		Code:    6003,
		Message: "Post id is required",
		Error:   "POST_ID_REQUIRED",
		Status:  http.StatusBadRequest,
	}
	OriginalCommentIdRequired = ErrorCode{
		Code:    6004,
		Message: "Original comment id is required",
		Error:   "ORIGINAL_COMMENT_ID_REQUIRED",
		Status:  http.StatusBadRequest,
	}
	LikedComment = ErrorCode{
		Code:    6005,
		Message: "Liked comment",
		Error:   "LIKED_COMMENT",
		Status:  http.StatusBadRequest,
	}
	NotLikedComment = ErrorCode{
		Code:    6006,
		Message: "Not liked comment",
		Error:   "NOT_LIKED_COMMENT",
		Status:  http.StatusBadRequest,
	}
	CanNotReplyRepliedComment = ErrorCode{
		Code:    6007,
		Message: "Can't reply replied comment",
		Error:   "CAN_NOT_REPLY_REPLIED_COMMENT",
		Status:  http.StatusBadRequest,
	}
	CommentContentUnchanged = ErrorCode{
		Code:    6008,
		Message: "Comment content unchanged",
		Error:   "COMMENT_CONTENT_UNCHANGED",
		Status:  http.StatusBadRequest,
	}

	// Chat errors (7000-7999)
	ChatNotFound = ErrorCode{
		Code:    7000,
		Message: "Chat not found",
		Error:   "CHAT_NOT_FOUND",
		Status:  http.StatusBadRequest,
	}
	MessageUsernameRequired = ErrorCode{
		Code:    7001,
		Message: "Username is required",
		Error:   "MESSAGE_USERNAME_REQUIRED",
		Status:  http.StatusBadRequest,
	}
	InvalidMessageContentLength = ErrorCode{
		Code:    7002,
		Message: "Invalid message content length",
		Error:   "INVALID_MESSAGE_CONTENT_LENGTH",
		Status:  http.StatusBadRequest,
	}
	ChatIdAndUserIdBothEmpty = ErrorCode{
		Code:    7003,
		Message: "Chat id and user id cannot be empty",
		Error:   "CHAT_ID_AND_USER_ID_BOTH_EMPTY",
		Status:  http.StatusBadRequest,
	}
	MessageNotFound = ErrorCode{
		Code:    7004,
		Message: "Message not found",
		Error:   "MESSAGE_NOT_FOUND",
		Status:  http.StatusBadRequest,
	}
	CanNotDeleteMessage = ErrorCode{
		Code:    7005,
		Message: "Can't delete message",
		Error:   "CAN_NOT_DELETE_MESSAGE",
		Status:  http.StatusBadRequest,
	}
	TextMessageContentRequired = ErrorCode{
		Code:    7006,
		Message: "Text message content is required",
		Error:   "TEXT_MESSAGE_CONTENT_REQUIRED",
		Status:  http.StatusBadRequest,
	}
	TextMessageContentUnchanged = ErrorCode{
		Code:    7007,
		Message: "Text message content unchanged",
		Error:   "TEXT_MESSAGE_CONTENT_UNCHANGED",
		Status:  http.StatusBadRequest,
	}
	CanNotEditFileMessage = ErrorCode{
		Code:    7008,
		Message: "Can't edit file message",
		Error:   "CAN_NOT_EDIT_FILE_MESSAGE",
		Status:  http.StatusBadRequest,
	}
	CanNotEditMessage = ErrorCode{
		Code:    7009,
		Message: "Can't edit message",
		Error:   "CAN_NOT_EDIT_MESSAGE",
		Status:  http.StatusBadRequest,
	}
	FileMessageRequired = ErrorCode{
		Code:    7010,
		Message: "File message is required",
		Error:   "FILE_MESSAGE_REQUIRED",
		Status:  http.StatusBadRequest,
	}
	AlreadyInCall = ErrorCode{
		Code:    7011,
		Message: "You are already in call",
		Error:   "ALREADY_IN_CALL",
		Status:  http.StatusBadRequest,
	}
	TargetAlreadyInCall = ErrorCode{
		Code:    7012,
		Message: "Target is already in call",
		Error:   "TARGET_ALREADY_IN_IN_CALL",
		Status:  http.StatusBadRequest,
	}
	CallNotFound = ErrorCode{
		Code:    7013,
		Message: "Call not found",
		Error:   "CALL_NOT_FOUND",
		Status:  http.StatusNotFound,
	}
	NotReadyForCall = ErrorCode{
		Code:    7014,
		Message: "Not ready for call",
		Error:   "NOT_READY_FOR_CALL",
		Status:  http.StatusBadRequest,
	}
	CanNotEditCall = ErrorCode{
		Code:    7015,
		Message: "Can't edit call",
		Error:   "CAN_NOT_EDIT_CALL",
		Status:  http.StatusBadRequest,
	}
	GifUrlRequired = ErrorCode{
		Code:    7016,
		Message: "Gif url is required",
		Error:   "GIF_URL_REQUIRED",
		Status:  http.StatusBadRequest,
	}

	// General/System errors (9000-9999)
	InvalidRequestMethod = ErrorCode{
		Code:    7015, // Mapped to 7015 in Java ErrorCode
		Message: "Invalid request method",
		Error:   "INVALID_REQUEST_METHOD",
		Status:  http.StatusMethodNotAllowed,
	}
	SearchQueryRequired = ErrorCode{
		Code:    9000,
		Message: "Search query is required",
		Error:   "SEARCH_QUERY_REQUIRED",
		Status:  http.StatusBadRequest,
	}
	TooManyRequests = ErrorCode{
		Code:    9991,
		Message: "Too many requests, please wait a minutes",
		Error:   "TOO_MANY_REQUESTS",
		Status:  http.StatusTooManyRequests,
	}
	InvalidWebsocketChannel = ErrorCode{
		Code:    9992,
		Message: "Invalid websocket channel",
		Error:   "INVALID_WEBSOCKET_CHANNEL",
		Status:  http.StatusBadRequest,
	}
	OnlyLetterAccepted = ErrorCode{
		Code:    9993,
		Message: "Only letter accepted",
		Error:   "ONLY_LETTER_ACCEPTED",
		Status:  http.StatusBadRequest,
	}
	Unauthorized = ErrorCode{
		Code:    9994,
		Message: "Unauthorized",
		Error:   "UNAUTHORIZED",
		Status:  http.StatusBadRequest,
	}
	InvalidInput = ErrorCode{
		Code:    9995,
		Message: "Invalid input",
		Error:   "INVALID_INPUT",
		Status:  http.StatusBadRequest,
	}
	InvalidUUID = ErrorCode{
		Code:    9996,
		Message: "Invalid uuid",
		Error:   "INVALID_UUID",
		Status:  http.StatusBadRequest,
	}
	Unauthenticated = ErrorCode{
		Code:    9997,
		Message: "Unauthenticated",
		Error:   "UNAUTHENTICATED",
		Status:  http.StatusUnauthorized,
	}
	NoResourceFound = ErrorCode{
		Code:    9998,
		Message: "Resource not found",
		Error:   "NO_RESOURCE_FOUND",
		Status:  http.StatusNotFound,
	}
	UnknownError = ErrorCode{
		Code:    9999,
		Message: "Something went wrong",
		Error:   "UNKNOWN_ERROR",
		Status:  http.StatusInternalServerError,
	}
)

// ToMap returns the error formatted as a WebSocket payload map
func (e ErrorCode) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"command": "ERROR",
		"code":    e.Code,
		"message": e.Message,
		"error":   e.Error,
	}
}

// Marshal converts the error map into standard JSON bytes for WebSocket transmission
func (e ErrorCode) Marshal() []byte {
	bytes, _ := json.Marshal(e.ToMap())
	return bytes
}

// AppException represents a strongly-typed centralized business exception carrying an ErrorCode,
// mimicking Java's AppException(ErrorCode) pattern.
type AppException struct {
	ErrCode ErrorCode
}

func (e AppException) Error() string {
	return e.ErrCode.Message
}

func NewAppException(errCode ErrorCode) AppException {
	return AppException{ErrCode: errCode}
}
