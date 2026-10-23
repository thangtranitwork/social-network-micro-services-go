import 'author_model.dart';

class PostModel {
  final String id;
  final String content;
  final String authorId;
  final AuthorModel? author;
  final DateTime createdAt;
  final String privacy;
  final int likeCount;
  final int commentCount;
  final int shareCount;
  final bool liked;
  final List<String> images;
  final List<String> files;

  PostModel({
    required this.id,
    required this.content,
    required this.authorId,
    this.author,
    required this.createdAt,
    this.privacy = 'PUBLIC',
    this.likeCount = 0,
    this.commentCount = 0,
    this.shareCount = 0,
    this.liked = false,
    this.images = const [],
    this.files = const [],
  });

  PostModel copyWith({
    String? id,
    String? content,
    String? authorId,
    AuthorModel? author,
    DateTime? createdAt,
    String? privacy,
    int? likeCount,
    int? commentCount,
    int? shareCount,
    bool? liked,
    List<String>? images,
    List<String>? files,
  }) {
    return PostModel(
      id: id ?? this.id,
      content: content ?? this.content,
      authorId: authorId ?? this.authorId,
      author: author ?? this.author,
      createdAt: createdAt ?? this.createdAt,
      privacy: privacy ?? this.privacy,
      likeCount: likeCount ?? this.likeCount,
      commentCount: commentCount ?? this.commentCount,
      shareCount: shareCount ?? this.shareCount,
      liked: liked ?? this.liked,
      images: images ?? this.images,
      files: files ?? this.files,
    );
  }

  factory PostModel.fromJson(Map<String, dynamic> json) {
    DateTime parsedDate;
    try {
      parsedDate = json['createdAt'] != null
          ? DateTime.parse(json['createdAt'])
          : DateTime.now();
    } catch (_) {
      parsedDate = DateTime.now();
    }

    List<String> parsedImages = [];
    if (json['images'] != null && json['images'] is List) {
      parsedImages = (json['images'] as List).map((e) => e.toString()).toList();
    }

    List<String> parsedFiles = [];
    if (json['files'] != null && json['files'] is List) {
      parsedFiles = (json['files'] as List).map((e) => e.toString()).toList();
    }

    return PostModel(
      id: json['id'] ?? '',
      content: json['content'] ?? '',
      authorId: json['authorId'] ?? json['author_id'] ?? json['userId'] ?? '',
      author: json['author'] != null ? AuthorModel.fromJson(json['author']) : null,
      createdAt: parsedDate,
      privacy: json['privacy'] ?? 'PUBLIC',
      likeCount: json['likeCount'] ?? json['like_count'] ?? 0,
      commentCount: json['commentCount'] ?? json['comment_count'] ?? 0,
      shareCount: json['shareCount'] ?? json['share_count'] ?? 0,
      liked: json['liked'] ?? false,
      images: parsedImages,
      files: parsedFiles,
    );
  }
}
