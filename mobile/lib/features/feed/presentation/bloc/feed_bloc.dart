import 'package:equatable/equatable.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import '../../data/models/post_model.dart';
import '../../data/repositories/feed_repository.dart';

// EVENTS
abstract class FeedEvent extends Equatable {
  const FeedEvent();
  @override
  List<Object?> get props => [];
}

class LoadFeedEvent extends FeedEvent {
  final bool isRefresh;
  const LoadFeedEvent({this.isRefresh = false});
  @override
  List<Object?> get props => [isRefresh];
}

class LoadMoreFeedEvent extends FeedEvent {}

class ToggleLikePostEvent extends FeedEvent {
  final String postId;
  const ToggleLikePostEvent(this.postId);
  @override
  List<Object?> get props => [postId];
}

// STATES
abstract class FeedState extends Equatable {
  const FeedState();
  @override
  List<Object?> get props => [];
}

class FeedInitialState extends FeedState {}

class FeedLoadingState extends FeedState {}

class FeedLoadedState extends FeedState {
  final List<PostModel> posts;
  final bool hasReachedMax;
  final bool isFetchingMore;

  const FeedLoadedState({
    required this.posts,
    this.hasReachedMax = false,
    this.isFetchingMore = false,
  });

  FeedLoadedState copyWith({
    List<PostModel>? posts,
    bool? hasReachedMax,
    bool? isFetchingMore,
  }) {
    return FeedLoadedState(
      posts: posts ?? this.posts,
      hasReachedMax: hasReachedMax ?? this.hasReachedMax,
      isFetchingMore: isFetchingMore ?? this.isFetchingMore,
    );
  }

  @override
  List<Object?> get props => [posts, hasReachedMax, isFetchingMore];
}

class FeedErrorState extends FeedState {
  final String message;
  const FeedErrorState(this.message);
  @override
  List<Object?> get props => [message];
}

// BLOC
class FeedBloc extends Bloc<FeedEvent, FeedState> {
  final FeedRepository feedRepository;
  static const int _pageSize = 10;

  FeedBloc({required this.feedRepository}) : super(FeedInitialState()) {
    on<LoadFeedEvent>(_onLoadFeed);
    on<LoadMoreFeedEvent>(_onLoadMoreFeed);
    on<ToggleLikePostEvent>(_onToggleLikePost);
  }

  Future<void> _onLoadFeed(LoadFeedEvent event, Emitter<FeedState> emit) async {
    if (!event.isRefresh) {
      emit(FeedLoadingState());
    }

    try {
      final posts = await feedRepository.getNewsfeed(skip: 0, limit: _pageSize);
      emit(FeedLoadedState(
        posts: posts,
        hasReachedMax: posts.length < _pageSize,
      ));
    } catch (e) {
      emit(FeedErrorState(e.toString().replaceAll('Exception: ', '')));
    }
  }

  Future<void> _onLoadMoreFeed(LoadMoreFeedEvent event, Emitter<FeedState> emit) async {
    final currentState = state;
    if (currentState is FeedLoadedState && !currentState.hasReachedMax && !currentState.isFetchingMore) {
      emit(currentState.copyWith(isFetchingMore: true));
      try {
        final morePosts = await feedRepository.getNewsfeed(
          skip: currentState.posts.length,
          limit: _pageSize,
        );

        if (morePosts.isEmpty) {
          emit(currentState.copyWith(hasReachedMax: true, isFetchingMore: false));
        } else {
          emit(currentState.copyWith(
            posts: List.of(currentState.posts)..addAll(morePosts),
            hasReachedMax: morePosts.length < _pageSize,
            isFetchingMore: false,
          ));
        }
      } catch (_) {
        emit(currentState.copyWith(isFetchingMore: false));
      }
    }
  }

  Future<void> _onToggleLikePost(ToggleLikePostEvent event, Emitter<FeedState> emit) async {
    final currentState = state;
    if (currentState is FeedLoadedState) {
      final updatedPosts = currentState.posts.map((post) {
        if (post.id == event.postId) {
          final isCurrentlyLiked = post.liked;
          final newLikeCount = isCurrentlyLiked ? (post.likeCount - 1) : (post.likeCount + 1);
          return post.copyWith(
            liked: !isCurrentlyLiked,
            likeCount: newLikeCount < 0 ? 0 : newLikeCount,
          );
        }
        return post;
      }).toList();

      emit(currentState.copyWith(posts: updatedPosts));

      // Make API call optimistically
      try {
        final target = currentState.posts.firstWhere((p) => p.id == event.postId);
        if (target.liked) {
          await feedRepository.unlikePost(event.postId);
        } else {
          await feedRepository.likePost(event.postId);
        }
      } catch (_) {
        // Revert on failure
        emit(currentState);
      }
    }
  }
}
