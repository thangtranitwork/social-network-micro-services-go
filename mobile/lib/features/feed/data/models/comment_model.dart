import 'author_model.dart';

class CommentModel {
  final String id;
  final String postId;
  final String userId;
  final String content;
  final DateTime createdAt;
  final AuthorModel? author;

  CommentModel({
    required this.id,
    required this.postId,
    required this.userId,
    required this.content,
    required this.createdAt,
    this.author,
  });

  factory CommentModel.fromJson(Map<String, dynamic> json) {
    DateTime parsedDate;
    try {
      parsedDate = json['createdAt'] != null
          ? DateTime.parse(json['createdAt'])
          : DateTime.now();
    } catch (_) {
      parsedDate = DateTime.now();
    }

    return CommentModel(
      id: json['id'] ?? '',
      postId: json['postId'] ?? json['post_id'] ?? '',
      userId: json['userId'] ?? json['user_id'] ?? '',
      content: json['content'] ?? '',
      createdAt: parsedDate,
      author: json['author'] != null ? AuthorModel.fromJson(json['author']) : null,
    );
  }
}
