import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:google_fonts/google_fonts.dart';
import '../../../../core/constants/app_colors.dart';
import '../bloc/feed_bloc.dart';
import '../widgets/post_card.dart';
import '../widgets/stories_bar.dart';

class FeedScreen extends StatefulWidget {
  const FeedScreen({super.key});

  @override
  State<FeedScreen> createState() => _FeedScreenState();
}

class _FeedScreenState extends State<FeedScreen> {
  final _scrollController = ScrollController();

  @override
  void initState() {
    super.initState();
    _scrollController.addListener(_onScroll);
  }

  @override
  void dispose() {
    _scrollController.dispose();
    super.dispose();
  }

  void _onScroll() {
    if (_scrollController.position.pixels >= _scrollController.position.maxScrollExtent - 300) {
      context.read<FeedBloc>().add(LoadMoreFeedEvent());
    }
  }

  @override
  Widget build(BuildContext context) {
    final isDark = Theme.of(context).brightness == Brightness.dark;

    return Scaffold(
      backgroundColor: isDark ? AppColors.darkBackground : AppColors.lightBackground,
      appBar: AppBar(
        backgroundColor: (isDark ? AppColors.darkBackground : AppColors.lightBackground).withOpacity(0.95),
        elevation: 0,
        scrolledUnderElevation: 0,
        titleSpacing: 18,
        title: Text(
          'PocPoc',
          style: GoogleFonts.outfit(
            fontSize: 26,
            fontWeight: FontWeight.w800,
            letterSpacing: -0.5,
            foreground: Paint()
              ..shader = AppColors.primaryGradient.createShader(
                const Rect.fromLTWH(0.0, 0.0, 200.0, 70.0),
              ),
          ),
        ),
        actions: [
          IconButton(
            icon: Icon(
              Icons.search_rounded,
              color: isDark ? AppColors.textPrimaryDark : AppColors.textPrimaryLight,
              size: 24,
            ),
            onPressed: () {},
          ),
          IconButton(
            icon: Icon(
              Icons.favorite_border_rounded,
              color: isDark ? AppColors.textPrimaryDark : AppColors.textPrimaryLight,
              size: 24,
            ),
            onPressed: () {},
          ),
          const SizedBox(width: 8),
        ],
      ),
      body: BlocBuilder<FeedBloc, FeedState>(
        builder: (context, state) {
          if (state is FeedLoadingState) {
            return const Center(
              child: CircularProgressIndicator(
                valueColor: AlwaysStoppedAnimation<Color>(AppColors.primary),
                strokeWidth: 2.8,
              ),
            );
          }

          if (state is FeedErrorState) {
            return Center(
              child: Column(
                mainAxisAlignment: MainAxisAlignment.center,
                children: [
                  Icon(
                    Icons.wifi_off_rounded,
                    size: 54,
                    color: isDark ? AppColors.textTertiaryDark : AppColors.textTertiaryLight,
                  ),
                  const SizedBox(height: 14),
                  Text(
                    state.message,
                    textAlign: TextAlign.center,
                    style: TextStyle(
                      color: isDark ? AppColors.textSecondaryDark : AppColors.textSecondaryLight,
                    ),
                  ),
                  const SizedBox(height: 18),
                  ElevatedButton(
                    onPressed: () {
                      context.read<FeedBloc>().add(const LoadFeedEvent(isRefresh: true));
                    },
                    style: ElevatedButton.styleFrom(
                      backgroundColor: AppColors.primary,
                      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
                    ),
                    child: const Text('Try Again', style: TextStyle(color: Colors.white)),
                  ),
                ],
              ),
            );
          }

          if (state is FeedLoadedState) {
            final posts = state.posts;

            return RefreshIndicator(
              color: AppColors.primary,
              backgroundColor: isDark ? AppColors.darkCard : AppColors.lightCard,
              onRefresh: () async {
                context.read<FeedBloc>().add(const LoadFeedEvent(isRefresh: true));
              },
              child: ListView.builder(
                controller: _scrollController,
                physics: const AlwaysScrollableScrollPhysics(
                  parent: BouncingScrollPhysics(),
                ),
                itemCount: posts.length + 2, // 1 for StoriesBar + posts + 1 for bottom loader
                itemBuilder: (context, index) {
                  if (index == 0) {
                    return const StoriesBar();
                  }

                  if (index == posts.length + 1) {
                    if (state.isFetchingMore) {
                      return const Padding(
                        padding: EdgeInsets.symmetric(vertical: 24),
                        child: Center(
                          child: CircularProgressIndicator(strokeWidth: 2.5),
                        ),
                      );
                    }
                    return const SizedBox(height: 80); // padding for floating navigation bar
                  }

                  final post = posts[index - 1];
                  return PostCard(post: post);
                },
              ),
            );
          }

          return const SizedBox.shrink();
        },
      ),
    );
  }
}
