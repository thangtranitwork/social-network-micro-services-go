import 'package:flutter/material.dart';
import '../../../../core/constants/app_colors.dart';
import '../../../../shared/widgets/user_avatar.dart';

class StoriesBar extends StatelessWidget {
  const StoriesBar({super.key});

  @override
  Widget build(BuildContext context) {
    final isDark = Theme.of(context).brightness == Brightness.dark;

    // Sample active stories users
    final storyUsers = [
      {'name': 'Obama', 'username': 'obama', 'hasStory': true},
      {'name': 'Trump', 'username': 'trump', 'hasStory': true},
      {'name': 'Biden', 'username': 'biden', 'hasStory': true},
      {'name': 'Alex', 'username': 'user_1', 'hasStory': true},
      {'name': 'Jessica', 'username': 'user_2', 'hasStory': true},
      {'name': 'David', 'username': 'user_3', 'hasStory': true},
    ];

    return Container(
      height: 104,
      padding: const EdgeInsets.symmetric(vertical: 8),
      child: ListView.separated(
        scrollDirection: Axis.horizontal,
        padding: const EdgeInsets.symmetric(horizontal: 16),
        itemCount: storyUsers.length + 1,
        separatorBuilder: (_, __) => const SizedBox(width: 14),
        itemBuilder: (context, index) {
          if (index == 0) {
            // Add Your Story Item
            return Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                Stack(
                  children: [
                    const UserAvatar(
                      name: 'You',
                      radius: 28,
                      hasStory: false,
                    ),
                    Positioned(
                      right: 0,
                      bottom: 0,
                      child: Container(
                        width: 20,
                        height: 20,
                        decoration: BoxDecoration(
                          gradient: AppColors.primaryGradient,
                          shape: BoxShape.circle,
                          border: Border.all(
                            color: isDark ? AppColors.darkBackground : Colors.white,
                            width: 2,
                          ),
                        ),
                        child: const Icon(Icons.add, size: 14, color: Colors.white),
                      ),
                    ),
                  ],
                ),
                const SizedBox(height: 6),
                Text(
                  'Your story',
                  style: TextStyle(
                    fontSize: 11,
                    fontWeight: FontWeight.w500,
                    color: isDark ? AppColors.textSecondaryDark : AppColors.textSecondaryLight,
                  ),
                ),
              ],
            );
          }

          final user = storyUsers[index - 1];
          return GestureDetector(
            onTap: () {
              // Open story viewer
            },
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                UserAvatar(
                  name: user['name'] as String,
                  radius: 28,
                  hasStory: user['hasStory'] as bool,
                ),
                const SizedBox(height: 6),
                Text(
                  user['name'] as String,
                  style: TextStyle(
                    fontSize: 11,
                    fontWeight: FontWeight.w500,
                    color: isDark ? AppColors.textPrimaryDark : AppColors.textPrimaryLight,
                  ),
                ),
              ],
            ),
          );
        },
      ),
    );
  }
}
