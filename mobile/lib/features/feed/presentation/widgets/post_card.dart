import 'package:cached_network_image/cached_network_image.dart';
import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import '../../../../core/constants/app_colors.dart';
import '../../../../shared/widgets/user_avatar.dart';
import '../../data/models/post_model.dart';
import '../bloc/feed_bloc.dart';
import 'comments_bottom_sheet.dart';

class PostCard extends StatefulWidget {
  final PostModel post;

  const PostCard({super.key, required this.post});

  @override
  State<PostCard> createState() => _PostCardState();
}

class _PostCardState extends State<PostCard> with SingleTickerProviderStateMixin {
  bool _showHeartAnimation = false;
  late AnimationController _heartAnimController;
  late Animation<double> _heartScale;

  @override
  void initState() {
    super.initState();
    _heartAnimController = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 350),
    );
    _heartScale = TweenSequence<double>([
      TweenSequenceItem(tween: Tween(begin: 0.0, end: 1.3).chain(CurveTween(curve: Curves.easeOutBack)), weight: 50),
      TweenSequenceItem(tween: Tween(begin: 1.3, end: 1.0).chain(CurveTween(curve: Curves.easeIn)), weight: 50),
    ]).animate(_heartAnimController);
  }

  @override
  void dispose() {
    _heartAnimController.dispose();
    super.dispose();
  }

  void _onDoubleTapLike() {
    if (!widget.post.liked) {
      context.read<FeedBloc>().add(ToggleLikePostEvent(widget.post.id));
    }
    setState(() {
      _showHeartAnimation = true;
    });
    _heartAnimController.forward(from: 0.0).then((_) {
      Future.delayed(const Duration(milliseconds: 400), () {
        if (mounted) {
          setState(() {
            _showHeartAnimation = false;
          });
        }
      });
    });
  }

  String _formatTimeAgo(DateTime dateTime) {
    final diff = DateTime.now().difference(dateTime);
    if (diff.inDays > 30) {
      return '${(diff.inDays / 30).floor()}mo';
    } else if (diff.inDays > 0) {
      return '${diff.inDays}d';
    } else if (diff.inHours > 0) {
      return '${diff.inHours}h';
    } else if (diff.inMinutes > 0) {
      return '${diff.inMinutes}m';
    }
    return 'now';
  }

  @override
  Widget build(BuildContext context) {
    final isDark = Theme.of(context).brightness == Brightness.dark;
    final post = widget.post;
    final authorName = post.author?.fullName.isNotEmpty == true
        ? post.author!.fullName
        : (post.author?.username ?? 'Anonymous');

    return Container(
      margin: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
      decoration: BoxDecoration(
        color: isDark ? AppColors.darkCard : AppColors.lightCard,
        borderRadius: BorderRadius.circular(24),
        border: Border.all(
          color: isDark ? AppColors.darkBorderSubtle : AppColors.lightBorder,
          width: 0.8,
        ),
        boxShadow: [
          BoxShadow(
            color: Colors.black.withOpacity(isDark ? 0.35 : 0.04),
            blurRadius: 18,
            offset: const Offset(0, 6),
          ),
        ],
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          // 1. Author Row
          Padding(
            padding: const EdgeInsets.fromLTRB(16, 14, 8, 10),
            child: Row(
              children: [
                UserAvatar(
                  imageUrl: post.author?.profilePictureUrl,
                  name: authorName,
                  radius: 20,
                  hasStory: false,
                ),
                const SizedBox(width: 12),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Row(
                        children: [
                          Flexible(
                            child: Text(
                              authorName,
                              overflow: TextOverflow.ellipsis,
                              style: TextStyle(
                                fontSize: 15,
                                fontWeight: FontWeight.w700,
                                letterSpacing: -0.2,
                                color: isDark ? AppColors.textPrimaryDark : AppColors.textPrimaryLight,
                              ),
                            ),
                          ),
                          const SizedBox(width: 4),
                          const Icon(
                            Icons.verified_rounded,
                            size: 14,
                            color: AppColors.primary,
                          ),
                        ],
                      ),
                      const SizedBox(height: 2),
                      Text(
                        '@${post.author?.username ?? "user"} • ${_formatTimeAgo(post.createdAt)}',
                        style: TextStyle(
                          fontSize: 12,
                          color: isDark ? AppColors.textSecondaryDark : AppColors.textSecondaryLight,
                        ),
                      ),
                    ],
                  ),
                ),
                IconButton(
                  icon: Icon(
                    Icons.more_horiz_rounded,
                    size: 20,
                    color: isDark ? AppColors.textTertiaryDark : AppColors.textTertiaryLight,
                  ),
                  onPressed: () {},
                ),
              ],
            ),
          ),

          // 2. Post Content Text
          if (post.content.isNotEmpty)
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 4),
              child: Text(
                post.content,
                style: TextStyle(
                  fontSize: 15,
                  height: 1.5,
                  color: isDark ? AppColors.textPrimaryDark : AppColors.textPrimaryLight,
                ),
              ),
            ),

          // 3. Post Media with Double-Tap to Like
          if (post.images.isNotEmpty || post.files.isNotEmpty) ...[
            const SizedBox(height: 12),
            GestureDetector(
              onDoubleTap: _onDoubleTapLike,
              child: Stack(
                alignment: Alignment.center,
                children: [
                  _buildMedia(post.images.isNotEmpty ? post.images : post.files),
                  if (_showHeartAnimation)
                    ScaleTransition(
                      scale: _heartScale,
                      child: Container(
                        padding: const EdgeInsets.all(20),
                        decoration: BoxDecoration(
                          shape: BoxShape.circle,
                          color: Colors.black.withOpacity(0.35),
                        ),
                        child: const Icon(
                          Icons.favorite_rounded,
                          size: 72,
                          color: AppColors.error,
                        ),
                      ),
                    ),
                ],
              ),
            ),
          ],

          const SizedBox(height: 12),

          // 4. Action Row (Instagram/Threads Style Floating Bar)
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 6),
            child: Row(
              children: [
                // Heart Like Button
                InkWell(
                  borderRadius: BorderRadius.circular(20),
                  onTap: () {
                    context.read<FeedBloc>().add(ToggleLikePostEvent(post.id));
                  },
                  child: Padding(
                    padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 6),
                    child: Row(
                      children: [
                        Icon(
                          post.liked ? Icons.favorite_rounded : Icons.favorite_border_rounded,
                          size: 22,
                          color: post.liked ? AppColors.error : (isDark ? AppColors.textSecondaryDark : AppColors.textSecondaryLight),
                        ),
                        if (post.likeCount > 0) ...[
                          const SizedBox(width: 6),
                          Text(
                            '${post.likeCount}',
                            style: TextStyle(
                              fontSize: 13,
                              fontWeight: FontWeight.w600,
                              color: post.liked ? AppColors.error : (isDark ? AppColors.textSecondaryDark : AppColors.textSecondaryLight),
                            ),
                          ),
                        ],
                      ],
                    ),
                  ),
                ),
                const SizedBox(width: 14),

                // Comment Button
                InkWell(
                  borderRadius: BorderRadius.circular(20),
                  onTap: () {
                    showModalBottomSheet(
                      context: context,
                      isScrollControlled: true,
                      backgroundColor: Colors.transparent,
                      builder: (_) => CommentsBottomSheet(postId: post.id),
                    );
                  },
                  child: Padding(
                    padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 6),
                    child: Row(
                      children: [
                        Icon(
                          Icons.chat_bubble_outline_rounded,
                          size: 20,
                          color: isDark ? AppColors.textSecondaryDark : AppColors.textSecondaryLight,
                        ),
                        if (post.commentCount > 0) ...[
                          const SizedBox(width: 6),
                          Text(
                            '${post.commentCount}',
                            style: TextStyle(
                              fontSize: 13,
                              fontWeight: FontWeight.w600,
                              color: isDark ? AppColors.textSecondaryDark : AppColors.textSecondaryLight,
                            ),
                          ),
                        ],
                      ],
                    ),
                  ),
                ),
                const SizedBox(width: 14),

                // Share Button
                InkWell(
                  borderRadius: BorderRadius.circular(20),
                  onTap: () {},
                  child: Padding(
                    padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 6),
                    child: Icon(
                      Icons.send_rounded,
                      size: 20,
                      color: isDark ? AppColors.textSecondaryDark : AppColors.textSecondaryLight,
                    ),
                  ),
                ),

                const Spacer(),

                // Bookmark / Save Post
                IconButton(
                  icon: Icon(
                    Icons.bookmark_border_rounded,
                    size: 22,
                    color: isDark ? AppColors.textSecondaryDark : AppColors.textSecondaryLight,
                  ),
                  onPressed: () {},
                ),
              ],
            ),
          ),
          const SizedBox(height: 6),
        ],
      ),
    );
  }

  Widget _buildMedia(List<String> mediaList) {
    if (mediaList.isEmpty) return const SizedBox.shrink();

    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 10),
      child: ClipRRect(
        borderRadius: BorderRadius.circular(18),
        child: CachedNetworkImage(
          imageUrl: mediaList.first,
          width: double.infinity,
          height: 280,
          fit: BoxFit.cover,
          placeholder: (context, url) => Container(
            height: 280,
            color: Colors.grey.withOpacity(0.12),
            child: const Center(
              child: CircularProgressIndicator(strokeWidth: 2),
            ),
          ),
          errorWidget: (context, url, error) => const SizedBox.shrink(),
        ),
      ),
    );
  }
}
