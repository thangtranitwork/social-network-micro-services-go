class ChatUserModel {
  final String id;
  final String username;
  final String givenName;
  final String familyName;
  final String? profilePictureUrl;
  final bool isOnline;

  ChatUserModel({
    required this.id,
    required this.username,
    required this.givenName,
    required this.familyName,
    this.profilePictureUrl,
    this.isOnline = false,
  });

  String get displayName {
    final full = '$givenName $familyName'.trim();
    return full.isNotEmpty ? full : username;
  }

  factory ChatUserModel.fromJson(Map<String, dynamic> json) {
    return ChatUserModel(
      id: json['id'] ?? '',
      username: json['username'] ?? '',
      givenName: json['givenName'] ?? json['given_name'] ?? '',
      familyName: json['familyName'] ?? json['family_name'] ?? '',
      profilePictureUrl: json['profilePictureUrl'] ?? json['profile_picture_url'],
      isOnline: json['isOnline'] ?? json['is_online'] ?? false,
    );
  }
}
