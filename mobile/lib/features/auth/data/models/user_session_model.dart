class UserSessionModel {
  final String userId;
  final String email;
  final String username;
  final String role;
  final String token;
  final String? refreshToken;

  UserSessionModel({
    required this.userId,
    required this.email,
    required this.username,
    required this.role,
    required this.token,
    this.refreshToken,
  });

  factory UserSessionModel.fromJson(Map<String, dynamic> json, {String? token, String? refreshToken}) {
    return UserSessionModel(
      userId: json['user_id'] ?? json['userId'] ?? json['id'] ?? '',
      email: json['email'] ?? '',
      username: json['username'] ?? '',
      role: json['role'] ?? 'USER',
      token: token ?? json['token'] ?? '',
      refreshToken: refreshToken ?? json['refreshToken'],
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'userId': userId,
      'email': email,
      'username': username,
      'role': role,
      'token': token,
      'refreshToken': refreshToken,
    };
  }
}
