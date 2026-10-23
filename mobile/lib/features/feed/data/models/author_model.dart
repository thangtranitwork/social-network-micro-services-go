class AuthorModel {
  final String id;
  final String username;
  final String givenName;
  final String familyName;
  final String? profilePictureUrl;

  AuthorModel({
    required this.id,
    required this.username,
    required this.givenName,
    required this.familyName,
    this.profilePictureUrl,
  });

  String get fullName => '$givenName $familyName'.trim();

  factory AuthorModel.fromJson(Map<String, dynamic> json) {
    return AuthorModel(
      id: json['id'] ?? '',
      username: json['username'] ?? '',
      givenName: json['givenName'] ?? json['given_name'] ?? '',
      familyName: json['familyName'] ?? json['family_name'] ?? '',
      profilePictureUrl: json['profilePictureUrl'] ?? json['profile_picture_url'],
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'id': id,
      'username': username,
      'givenName': givenName,
      'familyName': familyName,
      'profilePictureUrl': profilePictureUrl,
    };
  }
}
