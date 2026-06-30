package exception

import (
	"encoding/json"
	"net/http"
	"time"

	"social-network-go/logger"

	"github.com/gin-gonic/gin"
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

	LoginFailed = ErrorCode{
		Code:    1020,
		Message: "Login failed",
		Error:   "LOGIN_FAILED",
		Status:  http.StatusBadRequest,
	}
	RefreshFailed = ErrorCode{
		Code:    1021,
		Message: "Token refresh failed",
		Error:   "REFRESH_FAILED",
		Status:  http.StatusBadRequest,
	}
	ForgotPasswordFailed = ErrorCode{
		Code:    1022,
		Message: "Forgot password operation failed",
		Error:   "FORGOT_PASSWORD_FAILED",
		Status:  http.StatusBadRequest,
	}
	ResetPasswordFailed = ErrorCode{
		Code:    1023,
		Message: "Reset password failed",
		Error:   "RESET_PASSWORD_FAILED",
		Status:  http.StatusBadRequest,
	}
	ChangePasswordFailed = ErrorCode{
		Code:    1024,
		Message: "Change password failed",
		Error:   "CHANGE_PASSWORD_FAILED",
		Status:  http.StatusBadRequest,
	}
	RegisterFailed = ErrorCode{
		Code:    1025,
		Message: "Registration failed",
		Error:   "REGISTER_FAILED",
		Status:  http.StatusBadRequest,
	}
	VerifyFailed = ErrorCode{
		Code:    1026,
		Message: "Verification failed",
		Error:   "VERIFY_FAILED",
		Status:  http.StatusBadRequest,
	}
	ResendEmailFailed = ErrorCode{
		Code:    1027,
		Message: "Resend email failed",
		Error:   "RESEND_EMAIL_FAILED",
		Status:  http.StatusBadRequest,
	}
	TooManyEmailRequests = ErrorCode{
		Code:    1028,
		Message: "Too many email requests from this IP",
		Error:   "TOO_MANY_EMAIL_REQUESTS",
		Status:  http.StatusTooManyRequests,
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

	UpdateBioFailed = ErrorCode{
		Code:    2020,
		Message: "Update bio failed",
		Error:   "UPDATE_BIO_FAILED",
		Status:  http.StatusBadRequest,
	}
	UpdateBirthdateFailed = ErrorCode{
		Code:    2021,
		Message: "Update birthdate failed",
		Error:   "UPDATE_BIRTHDATE_FAILED",
		Status:  http.StatusBadRequest,
	}
	UpdateNameFailed = ErrorCode{
		Code:    2022,
		Message: "Update name failed",
		Error:   "UPDATE_NAME_FAILED",
		Status:  http.StatusBadRequest,
	}
	UpdateUsernameFailed = ErrorCode{
		Code:    2023,
		Message: "Update username failed",
		Error:   "UPDATE_USERNAME_FAILED",
		Status:  http.StatusBadRequest,
	}
	UpdateProfilePictureFailed = ErrorCode{
		Code:    2024,
		Message: "Update profile picture failed",
		Error:   "UPDATE_PROFILE_PICTURE_FAILED",
		Status:  http.StatusBadRequest,
	}
	GetUserProfileFailed = ErrorCode{
		Code:    2025,
		Message: "Get user profile failed",
		Error:   "GET_USER_PROFILE_FAILED",
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
	SendFriendRequestFailed = ErrorCode{
		Code:    4001,
		Message: "Send friend request failed",
		Error:   "SEND_FRIEND_REQUEST_FAILED",
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
	DeleteRequestFailed = ErrorCode{
		Code:    4013,
		Message: "Delete friend request failed",
		Error:   "DELETE_REQUEST_FAILED",
		Status:  http.StatusBadRequest,
	}
	GetRequestsFailed = ErrorCode{
		Code:    4014,
		Message: "Get friend requests failed",
		Error:   "GET_REQUESTS_FAILED",
		Status:  http.StatusBadRequest,
	}
	UnfriendFailed = ErrorCode{
		Code:    4015,
		Message: "Unfriend failed",
		Error:   "UNFRIEND_FAILED",
		Status:  http.StatusBadRequest,
	}
	BlockFailed = ErrorCode{
		Code:    4016,
		Message: "Block user failed",
		Error:   "BLOCK_FAILED",
		Status:  http.StatusBadRequest,
	}
	UnblockFailed = ErrorCode{
		Code:    4017,
		Message: "Unblock user failed",
		Error:   "UNBLOCK_FAILED",
		Status:  http.StatusBadRequest,
	}
	GetBlockedUsersFailed = ErrorCode{
		Code:    4018,
		Message: "Get blocked users failed",
		Error:   "GET_BLOCKED_USERS_FAILED",
		Status:  http.StatusBadRequest,
	}
	GetFriendsFailed = ErrorCode{
		Code:    4019,
		Message: "Get friends failed",
		Error:   "GET_FRIENDS_FAILED",
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
	FailToGetPost = ErrorCode{
		Code:    5012,
		Message: "Fail to get post",
		Error:   "FAIL_TO_GET_POST",
		Status:  http.StatusBadRequest,
	}
	InvalidPostPrivacy = ErrorCode{
		Code:    5013,
		Message: "Invalid post privacy",
		Error:   "INVALID_POST_PRIVACY",
		Status:  http.StatusBadRequest,
	}
	CreatePostFailed = ErrorCode{
		Code:    5014,
		Message: "Create post failed",
		Error:   "CREATE_POST_FAILED",
		Status:  http.StatusBadRequest,
	}
	SharePostFailed = ErrorCode{
		Code:    5015,
		Message: "Share post failed",
		Error:   "SHARE_POST_FAILED",
		Status:  http.StatusBadRequest,
	}
	LikePostFailed = ErrorCode{
		Code:    5016,
		Message: "Like post failed",
		Error:   "LIKE_POST_FAILED",
		Status:  http.StatusBadRequest,
	}
	UnlikePostFailed = ErrorCode{
		Code:    5017,
		Message: "Unlike post failed",
		Error:   "UNLIKE_POST_FAILED",
		Status:  http.StatusBadRequest,
	}
	UpdatePrivacyFailed = ErrorCode{
		Code:    5018,
		Message: "Update post privacy failed",
		Error:   "UPDATE_PRIVACY_FAILED",
		Status:  http.StatusBadRequest,
	}
	UpdatePostFailed = ErrorCode{
		Code:    5019,
		Message: "Update post failed",
		Error:   "UPDATE_POST_FAILED",
		Status:  http.StatusBadRequest,
	}
	DeletePostFailed = ErrorCode{
		Code:    5020,
		Message: "Delete post failed",
		Error:   "DELETE_POST_FAILED",
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

	CommentFailed = ErrorCode{
		Code:    6010,
		Message: "Comment failed",
		Error:   "COMMENT_FAILED",
		Status:  http.StatusBadRequest,
	}
	ReplyCommentFailed = ErrorCode{
		Code:    6011,
		Message: "Reply comment failed",
		Error:   "REPLY_COMMENT_FAILED",
		Status:  http.StatusBadRequest,
	}
	LikeCommentFailed = ErrorCode{
		Code:    6012,
		Message: "Like comment failed",
		Error:   "LIKE_COMMENT_FAILED",
		Status:  http.StatusBadRequest,
	}
	UnlikeCommentFailed = ErrorCode{
		Code:    6013,
		Message: "Unlike comment failed",
		Error:   "UNLIKE_COMMENT_FAILED",
		Status:  http.StatusBadRequest,
	}
	GetCommentsFailed = ErrorCode{
		Code:    6014,
		Message: "Get comments failed",
		Error:   "GET_COMMENTS_FAILED",
		Status:  http.StatusBadRequest,
	}
	DeleteCommentFailed = ErrorCode{
		Code:    6015,
		Message: "Delete comment failed",
		Error:   "DELETE_COMMENT_FAILED",
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
	SendMessageFailed = ErrorCode{
		Code:    7018,
		Message: "Send message failed",
		Error:   "SEND_MESSAGE_FAILED",
		Status:  http.StatusBadRequest,
	}
	SendGifFailed = ErrorCode{
		Code:    7019,
		Message: "Send gif failed",
		Error:   "SEND_GIF_FAILED",
		Status:  http.StatusBadRequest,
	}
	SendFileFailed = ErrorCode{
		Code:    7020,
		Message: "Send file message failed",
		Error:   "SEND_FILE_FAILED",
		Status:  http.StatusBadRequest,
	}
	SendVoiceFailed = ErrorCode{
		Code:    7021,
		Message: "Send voice message failed",
		Error:   "SEND_VOICE_FAILED",
		Status:  http.StatusBadRequest,
	}
	EditMessageFailed = ErrorCode{
		Code:    7022,
		Message: "Edit message failed",
		Error:   "EDIT_MESSAGE_FAILED",
		Status:  http.StatusBadRequest,
	}
	DeleteMessageFailed = ErrorCode{
		Code:    7023,
		Message: "Delete message failed",
		Error:   "DELETE_MESSAGE_FAILED",
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

func NewAppError(code int, msg string, err string) ErrorCode {
	return ErrorCode{
		Code:    code,
		Message: msg,
		Error:   err,
		Status:  http.StatusBadRequest,
	}
}

var errorStringMap = map[string]ErrorCode{
	// Account errors
	"ACCOUNT_NOT_FOUND":                              AccountNotFound,
	"ACCOUNT_NOT_VERIFIED":                           AccountNotVerified,
	"ACCOUNT_NOT_VERIZED":                            AccountNotVerified,
	"ACCOUNT_LOCKED":                                 AccountLocked,
	"AUTHENTICATION_FAILED":                          AuthenticationFailed,
	"INVALID_PASSWORD":                               InvalidPassword,
	"WEAK_PASSWORD_MUST_BE_8_CHARS_UPPERCASE_NUMBER": InvalidPassword,
	"PASSWORD_NOT_STRONG_ENOUGH":                     InvalidPassword,
	"INVALID_EMAIL":                                  InvalidEmail,
	"EMAIL_REQUIRED":                                 EmailRequired,
	"PASSWORD_REQUIRED":                              PasswordRequired,
	"VERIFICATION_CODE_NOT_FOUND":                    VerificationCodeNotFound,
	"VERIFICATION_CODE_NOT_MATCHED_OR_EXPIRED":       VerificationCodeNotMatchedOrExpired,
	"REFRESH_TOKEN_REQUIRED":                         RefreshTokenRequired,
	"INVALID_OR_EXPIRED_REFRESH_TOKEN":               InvalidOrExpiredRefreshToken,
	"ACCOUNT_ALREADY_EXISTS":                         AccountAlreadyExists,
	"INVALID_TOKEN":                                  InvalidToken,
	"EXPIRED_TOKEN":                                  ExpiredToken,
	"VERIFICATION_CODE_REQUIRED":                     VerificationCodeRequired,
	"ACCOUNT_VERIFIED":                               AccountVerified,
	"ACCOUNT_ALREADY_VERIFIED":                       AccountVerified,
	"LOGIN_FAILED":                                   LoginFailed,
	"REFRESH_FAILED":                                 RefreshFailed,
	"FORGOT_PASSWORD_FAILED":                         ForgotPasswordFailed,
	"RESET_PASSWORD_FAILED":                          ResetPasswordFailed,
	"CHANGE_PASSWORD_FAILED":                         ChangePasswordFailed,
	"REGISTER_FAILED":                                RegisterFailed,
	"VERIFY_FAILED":                                  VerifyFailed,
	"RESEND_EMAIL_FAILED":                            ResendEmailFailed,
	"TOO_MANY_EMAIL_REQUESTS":                        TooManyEmailRequests,

	// User errors
	"GIVEN_NAME_REQUIRED":                           GivenNameRequired,
	"FAMILY_NAME_REQUIRED":                          FamilyNameRequired,
	"BIRTHDATE_REQUIRED":                            BirthdateRequired,
	"AGE_MUST_BE_AT_LEAST_16":                       AgeMustBeAtLeast16,
	"INVALID_GIVEN_NAME_LENGTH":                     InvalidGivenNameLength,
	"INVALID_FAMILY_NAME_LENGTH":                    InvalidFamilyNameLength,
	"LESS_THAN_30_DAYS_SINCE_LAST_BIRTHDATE_CHANGE": LessThan30DaysSinceLastBirthdateChange,
	"LESS_THAN_30_DAYS_SINCE_LAST_NAME_CHANGE":      LessThan30DaysSinceLastNameChange,
	"LESS_THAN_30_DAYS_SINCE_LAST_USERNAME_CHANGE":  LessThan30DaysSinceLastUsernameChange,
	"EMAIL_NOT_VERIFIED":                            EmailNotVerified,
	"USER_NOT_FOUND":                                UserNotFound,
	"INVALID_USERNAME":                              InvalidUsername,
	"USERNAME_REQUIRED":                             UsernameRequired,
	"USERNAME_ALREADY_EXISTS":                       UsernameAlreadyExists,
	"PROFILE_PICTURE_REQUIRED":                      ProfilePictureRequired,
	"NOTHING_CHANGED":                               NothingChanged,
	"UPDATE_BIO_FAILED":                             UpdateBioFailed,
	"UPDATE_BIRTHDATE_FAILED":                       UpdateBirthdateFailed,
	"UPDATE_NAME_FAILED":                            UpdateNameFailed,
	"UPDATE_USERNAME_FAILED":                        UpdateUsernameFailed,
	"UPDATE_PROFILE_PICTURE_FAILED":                 UpdateProfilePictureFailed,
	"GET_USER_PROFILE_FAILED":                       GetUserProfileFailed,

	// Storage errors
	"STORAGE_INITIALIZATION_ERROR": StorageInitializationError,
	"FILE_REQUIRED":                FileRequired,
	"INVALID_FILE":                 InvalidFile,
	"UPLOAD_FILE_FAILED":           UploadFileFailed,
	"FILE_NOT_FOUND":               FileNotFound,
	"DELETE_FILE_FAILED":           DeleteFileFailed,
	"LOAD_FILE_FAILED":             LoadFileFailed,
	"REQUIRED_IMAGE_FILE":          RequiredImageFile,
	"INVALID_FILE_SIZE":            InvalidFileSize,
	"LIST_CONTAINS_INVALID_FILE":   ListContainsInvalidFile,
	"REQUIRED_IMAGE_OR_VIDEO_FILE": RequiredImageOrVideoFile,

	// Friend/Block errors
	"CAN_NOT_MAKE_SELF_REQUEST":                 CanNotMakeSelfRequest,
	"SENT_ADD_FRIEND_REQUEST_FAILED":            SendFriendRequestFailed,
	"SEND_FRIEND_REQUEST_FAILED":                SendFriendRequestFailed,
	"ADD_FRIEND_REQUEST_SENT_LIMIT_REACHED":     AddFriendRequestSentLimitReached,
	"ADD_FRIEND_REQUEST_RECEIVED_LIMIT_REACHED": AddFriendRequestReceivedLimitReached,
	"REQUEST_NOT_FOUND":                         RequestNotFound,
	"ACCEPT_REQUEST_FAILED":                     AcceptRequestFailed,
	"HAS_BLOCKED":                               HasBlocked,
	"HAS_BEEN_BLOCKED":                          HasBeenBlocked,
	"NOT_BLOCK":                                 NotBlock,
	"BLOCK_LIMIT_REACHED":                       BlockLimitReached,
	"CAN_NOT_BLOCK_YOURSELF":                    CanNotBlockYourself,
	"BLOCK_NOT_FOUND":                           BlockNotFound,
	"FRIEND_NOT_FOUND":                          FriendNotFound,
	"DELETE_REQUEST_FAILED":                     DeleteRequestFailed,
	"GET_REQUESTS_FAILED":                       GetRequestsFailed,
	"UNFRIEND_FAILED":                           UnfriendFailed,
	"BLOCK_FAILED":                              BlockFailed,
	"UNBLOCK_FAILED":                            UnblockFailed,
	"GET_BLOCKED_USERS_FAILED":                  GetBlockedUsersFailed,
	"GET_FRIENDS_FAILED":                        GetFriendsFailed,

	// Post errors
	"POST_CONTENT_AND_ATTACH_FILES_BOTH_EMPTY": PostContentAndAttachFilesBothEmpty,
	"INVALID_POST_CONTENT_LENGTH":              InvalidPostContentLength,
	"INVALID_NUMBER_OF_POST_ATTACHMENTS":       InvalidNumberOfPostAttachments,
	"POST_NOT_FOUND":                           PostNotFound,
	"ONLY_PUBLIC_POST_CAN_BE_SHARED":           OnlyPublicPostCanBeShared,
	"PRIVACY_UNCHANGED":                        PrivacyUnchanged,
	"INVALID_DELETE_ATTACHMENT":                InvalidDeleteAttachment,
	"POST_CONTENT_UNCHANGED":                   PostContentUnchanged,
	"LIKED_POST":                               LikedPost,
	"NOT_LIKED_POST":                           NotLikedPost,
	"DELETED_POST":                             DeletedPost,
	"INVALID_POST_PRIVACY":                     InvalidPostPrivacy,
	"CREATE_POST_FAILED":                       CreatePostFailed,
	"SHARE_POST_FAILED":                        SharePostFailed,
	"LIKE_POST_FAILED":                         LikePostFailed,
	"UNLIKE_POST_FAILED":                       UnlikePostFailed,
	"UPDATE_PRIVACY_FAILED":                    UpdatePrivacyFailed,
	"UPDATE_POST_FAILED":                       UpdatePostFailed,
	"DELETE_POST_FAILED":                       DeletePostFailed,

	// Comment errors
	"COMMENT_NOT_FOUND":                          CommentNotFound,
	"COMMENT_CONTENT_AND_ATTACH_FILE_BOTH_EMPTY": CommentContentAndAttachFileBothEmpty,
	"INVALID_COMMENT_CONTENT_LENGTH":             InvalidCommentContentLength,
	"POST_ID_REQUIRED":                           PostIdRequired,
	"ORIGINAL_COMMENT_ID_REQUIRED":               OriginalCommentIdRequired,
	"LIKED_COMMENT":                              LikedComment,
	"NOT_LIKED_COMMENT":                          NotLikedComment,
	"CAN_NOT_REPLY_REPLIED_COMMENT":              CanNotReplyRepliedComment,
	"COMMENT_CONTENT_UNCHANGED":                  CommentContentUnchanged,
	"COMMENT_FAILED":                             CommentFailed,
	"REPLY_COMMENT_FAILED":                       ReplyCommentFailed,
	"LIKE_COMMENT_FAILED":                        LikeCommentFailed,
	"UNLIKE_COMMENT_FAILED":                      UnlikeCommentFailed,
	"GET_COMMENTS_FAILED":                        GetCommentsFailed,
	"DELETE_COMMENT_FAILED":                      DeleteCommentFailed,

	// Chat errors
	"CHAT_NOT_FOUND":                 ChatNotFound,
	"MESSAGE_USERNAME_REQUIRED":      MessageUsernameRequired,
	"INVALID_MESSAGE_CONTENT_LENGTH": InvalidMessageContentLength,
	"CHAT_ID_AND_USER_ID_BOTH_EMPTY": ChatIdAndUserIdBothEmpty,
	"MESSAGE_NOT_FOUND":              MessageNotFound,
	"CAN_NOT_DELETE_MESSAGE":         CanNotDeleteMessage,
	"TEXT_MESSAGE_CONTENT_REQUIRED":  TextMessageContentRequired,
	"TEXT_MESSAGE_CONTENT_UNCHANGED": TextMessageContentUnchanged,
	"CAN_NOT_EDIT_FILE_MESSAGE":      CanNotEditFileMessage,
	"CAN_NOT_EDIT_MESSAGE":           CanNotEditMessage,
	"FILE_MESSAGE_REQUIRED":          FileMessageRequired,
	"ALREADY_IN_CALL":                AlreadyInCall,
	"TARGET_ALREADY_IN_IN_CALL":      TargetAlreadyInCall,
	"CALL_NOT_FOUND":                 CallNotFound,
	"NOT_READY_FOR_CALL":             NotReadyForCall,
	"CAN_NOT_EDIT_CALL":              CanNotEditCall,
	"GIF_URL_REQUIRED":               GifUrlRequired,
	"BLOCKED":                        HasBlocked,
	"CAN_NOT_EDIT_FILE_OR_CALL":      CanNotEditFileMessage,
	"SEND_MESSAGE_FAILED":            SendMessageFailed,
	"SEND_GIF_FAILED":                SendGifFailed,
	"SEND_FILE_FAILED":               SendFileFailed,
	"SEND_VOICE_FAILED":              SendVoiceFailed,
	"EDIT_MESSAGE_FAILED":            EditMessageFailed,
	"DELETE_MESSAGE_FAILED":          DeleteMessageFailed,

	// General/System
	"INVALID_REQUEST_METHOD":                      InvalidRequestMethod,
	"SEARCH_QUERY_REQUIRED":                       SearchQueryRequired,
	"TOO_MANY_REQUESTS":                           TooManyRequests,
	"INVALID_WEBSOCKET_CHANNEL":                   InvalidWebsocketChannel,
	"ONLY_LETTER_ACCEPTED":                        OnlyLetterAccepted,
	"UNAUTHORIZED":                                Unauthorized,
	"INVALID_INPUT":                               InvalidInput,
	"INVALID_REQUEST_PAYLOAD":                     InvalidInput,
	"INVALID_BIRTHDATE_FORMAT_MUST_BE_YYYY_MM_DD": InvalidInput,
	"INVALID_CODE_FORMAT":                         InvalidInput,
	"INVALID_REQUEST_BODY":                        InvalidInput,
	"INVALID_UUID":                                InvalidUUID,
	"UNAUTHENTICATED":                             Unauthenticated,
	"NO_RESOURCE_FOUND":                           NoResourceFound,
	"UNKNOWN_ERROR":                               UnknownError,
}

// MapError looks up the given error message and maps it to a standard ErrorCode.
// It supports direct matches and prefix-based matches (for auth locked/attempt states).
func MapError(msg string) (ErrorCode, bool) {
	if code, ok := errorStringMap[msg]; ok {
		return code, true
	}
	// Prefix checks for authentication failures/account lockouts
	if len(msg) >= 21 && msg[:21] == "AUTHENTICATION_FAILED" {
		return AuthenticationFailed, true
	}
	if len(msg) >= 14 && msg[:14] == "ACCOUNT_LOCKED" {
		return AccountLocked, true
	}
	return UnknownError, false
}

// MapAppError maps any error (including AppException and mapped string errors) to a standard ErrorCode.
func MapAppError(err error) (ErrorCode, bool) {
	if err == nil {
		return UnknownError, false
	}
	if appErr, ok := err.(AppException); ok {
		return appErr.ErrCode, true
	}
	if appErr, ok := err.(*AppException); ok && appErr != nil {
		return appErr.ErrCode, true
	}
	if code, ok := errorStringMap[err.Error()]; ok {
		return code, true
	}
	// Prefix checks for authentication failures/account lockouts
	msg := err.Error()
	if len(msg) >= 21 && msg[:21] == "AUTHENTICATION_FAILED" {
		return AuthenticationFailed, true
	}
	if len(msg) >= 14 && msg[:14] == "ACCOUNT_LOCKED" {
		return AccountLocked, true
	}
	return UnknownError, false
}

// SendError writes a JSON response for the given ErrorCode.
func SendError(c *gin.Context, errCode ErrorCode) {
	c.JSON(errCode.Status, gin.H{
		"code":      errCode.Code,
		"message":   errCode.Message,
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// SendAppError checks if err is an AppException, otherwise maps its message using MapError,
// and if no mapping is found, logs the internal error and returns UnknownError.
func SendAppError(c *gin.Context, err error) {
	if appErr, ok := err.(AppException); ok {
		SendError(c, appErr.ErrCode)
		return
	}
	if mapped, found := MapError(err.Error()); found {
		SendError(c, mapped)
		return
	}

	// Unmapped internal/system error - log it and return generic UnknownError
	logger.Err(err).Error("Unhandled application error")
	SendError(c, UnknownError)
}

// SendErrorWithStatus maps a string/magic string to standard ErrorCode if possible,
// otherwise uses the provided HTTP status, custom code, and message.
// If the message is not mapped, it logs it as a warning.
func SendErrorWithStatus(c *gin.Context, httpStatus int, code int, msg string) {
	if mapped, found := MapError(msg); found {
		SendError(c, mapped)
		return
	}

	logger.Field("status", httpStatus).Field("code", code).Warn("Unmapped handler error: %s", msg)
	c.JSON(httpStatus, gin.H{
		"code":      code,
		"message":   msg,
		"timestamp": time.Now().Format(time.RFC3339),
	})
}
