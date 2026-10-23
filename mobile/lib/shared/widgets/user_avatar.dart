import 'package:cached_network_image/cached_network_image.dart';
import 'package:flutter/material.dart';
import '../../core/constants/app_colors.dart';

class UserAvatar extends StatelessWidget {
  final String? imageUrl;
  final String? name;
  final double radius;
  final bool isOnline;
  final bool hasStory;
  final VoidCallback? onTap;

  const UserAvatar({
    super.key,
    this.imageUrl,
    this.name,
    this.radius = 22,
    this.isOnline = false,
    this.hasStory = false,
    this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    final initials = _getInitials(name);
    final isDark = Theme.of(context).brightness == Brightness.dark;

    Widget coreAvatar = CircleAvatar(
      radius: radius,
      backgroundColor: AppColors.primary.withOpacity(0.18),
      child: (imageUrl != null && imageUrl!.isNotEmpty)
          ? ClipRRect(
              borderRadius: BorderRadius.circular(radius),
              child: CachedNetworkImage(
                imageUrl: imageUrl!,
                width: radius * 2,
                height: radius * 2,
                fit: BoxFit.cover,
                placeholder: (context, url) => Container(
                  color: isDark ? AppColors.darkCardElevated : AppColors.lightCardElevated,
                ),
                errorWidget: (context, url, error) => _buildInitialsText(initials),
              ),
            )
          : _buildInitialsText(initials),
    );

    // If has story, wrap in luxury gradient ring
    if (hasStory) {
      coreAvatar = Container(
        padding: const EdgeInsets.all(2.5),
        decoration: const BoxDecoration(
          shape: BoxShape.circle,
          gradient: AppColors.storyGradient,
        ),
        child: Container(
          padding: const EdgeInsets.all(2),
          decoration: BoxDecoration(
            shape: BoxShape.circle,
            color: isDark ? AppColors.darkBackground : AppColors.lightBackground,
          ),
          child: coreAvatar,
        ),
      );
    }

    if (isOnline) {
      coreAvatar = Stack(
        clipBehavior: Clip.none,
        children: [
          coreAvatar,
          Positioned(
            right: hasStory ? 4 : 1,
            bottom: hasStory ? 4 : 1,
            child: Container(
              width: radius * 0.52,
              height: radius * 0.52,
              decoration: BoxDecoration(
                color: AppColors.onlineGreen,
                shape: BoxShape.circle,
                border: Border.all(
                  color: isDark ? AppColors.darkBackground : Colors.white,
                  width: 2,
                ),
                boxShadow: [
                  BoxShadow(
                    color: AppColors.onlineGreen.withOpacity(0.4),
                    blurRadius: 4,
                  ),
                ],
              ),
            ),
          ),
        ],
      );
    }

    if (onTap != null) {
      return GestureDetector(
        onTap: onTap,
        child: coreAvatar,
      );
    }

    return coreAvatar;
  }

  Widget _buildInitialsText(String initials) {
    return Text(
      initials,
      style: TextStyle(
        fontSize: radius * 0.78,
        fontWeight: FontWeight.w700,
        color: AppColors.primaryLight,
      ),
    );
  }

  String _getInitials(String? text) {
    if (text == null || text.trim().isEmpty) return '?';
    final parts = text.trim().split(' ');
    if (parts.length >= 2) {
      return '${parts[0][0]}${parts[parts.length - 1][0]}'.toUpperCase();
    }
    return text.substring(0, text.length >= 2 ? 2 : 1).toUpperCase();
  }
}
