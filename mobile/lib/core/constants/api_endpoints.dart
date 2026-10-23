class ApiEndpoints {
  // Base Gateway Address
  static const String defaultBaseUrl = 'http://10.0.2.2:11111'; // Android emulator default
  static const String defaultWsUrl = 'ws://10.0.2.2:11111';

  // 1. Auth Endpoints
  static const String login = '/v1/auth/login';
  static const String register = '/v1/register';
  static const String verifyOtp = '/v1/register/verify-otp';
  static const String refreshToken = '/v1/auth/refresh';
  static const String logout = '/v1/auth/logout';
  static const String forgotPassword = '/v1/auth/forgot-password';
  static const String resetPassword = '/v1/auth/reset-password';
  static const String changePassword = '/v1/auth/change-password';

  // 2. User & Social Graph
  static const String myProfile = '/v1/users/me';
  static const String userProfile = '/v1/users/profile';
  static const String updateProfile = '/v1/users/profile';
  static const String friendsList = '/v1/friends';
  static const String suggestedFriends = '/v1/friends/suggested';
  static const String friendRequests = '/v1/friend-request';
  static const String blocks = '/v1/blocks';

  // 3. Post & Feed Endpoints
  static const String newsfeed = '/v1/posts/newsfeed';
  static const String userPosts = '/v1/posts/user';
  static const String createPost = '/v1/posts';
  static const String postDetails = '/v1/posts';
  static const String postReactions = '/v1/posts/reactions';
  static const String postComments = '/v1/comments';

  // 4. Stories Endpoints
  static const String stories = '/v1/stories';
  static const String createStory = '/v1/stories';
  static const String viewStory = '/v1/stories/view';

  // 5. Chat & Call Endpoints
  static const String chatList = '/v1/chat';
  static const String chatMessages = '/v1/chat/messages';
  static const String chatGroups = '/v1/chat/groups';
  static const String chatWs = '/v1/chat/ws';
  static const String iceServers = '/v1/call/ice-servers';

  // 6. File & Media Endpoints
  static const String uploadFile = '/v1/files/upload';
  static const String presignedUpload = '/v1/files/upload/presigned';
  static const String fileDownload = '/v1/files';

  // 7. Notification & FCM Endpoints
  static const String notifications = '/v1/notifications';
  static const String markNotificationsRead = '/v1/notifications/read-all';
  static const String registerFcmToken = '/v1/fcm/register';

  // 8. Search Endpoints
  static const String searchUsers = '/v1/search/users';
  static const String searchPosts = '/v1/search/posts';
}
