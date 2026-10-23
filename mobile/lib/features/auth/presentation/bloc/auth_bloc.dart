import 'package:equatable/equatable.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import '../../data/models/user_session_model.dart';
import '../../data/repositories/auth_repository.dart';

// EVENTS
abstract class AuthEvent extends Equatable {
  const AuthEvent();
  @override
  List<Object?> get props => [];
}

class CheckAuthEvent extends AuthEvent {}

class LoginSubmittedEvent extends AuthEvent {
  final String email;
  final String password;

  const LoginSubmittedEvent({required this.email, required this.password});

  @override
  List<Object?> get props => [email, password];
}

class RegisterSubmittedEvent extends AuthEvent {
  final String email;
  final String username;
  final String password;
  final String givenName;
  final String familyName;

  const RegisterSubmittedEvent({
    required this.email,
    required this.username,
    required this.password,
    required this.givenName,
    required this.familyName,
  });

  @override
  List<Object?> get props => [email, username, password, givenName, familyName];
}

class LogoutEvent extends AuthEvent {}

// STATES
abstract class AuthState extends Equatable {
  const AuthState();
  @override
  List<Object?> get props => [];
}

class AuthInitialState extends AuthState {}

class AuthLoadingState extends AuthState {}

class AuthenticatedState extends AuthState {
  final UserSessionModel session;

  const AuthenticatedState(this.session);

  @override
  List<Object?> get props => [session];
}

class UnauthenticatedState extends AuthState {}

class AuthFailureState extends AuthState {
  final String errorMessage;

  const AuthFailureState(this.errorMessage);

  @override
  List<Object?> get props => [errorMessage];
}

class RegisterSuccessState extends AuthState {
  final String message;

  const RegisterSuccessState(this.message);

  @override
  List<Object?> get props => [message];
}

// BLOC
class AuthBloc extends Bloc<AuthEvent, AuthState> {
  final AuthRepository authRepository;

  AuthBloc({required this.authRepository}) : super(AuthInitialState()) {
    on<CheckAuthEvent>(_onCheckAuth);
    on<LoginSubmittedEvent>(_onLoginSubmitted);
    on<RegisterSubmittedEvent>(_onRegisterSubmitted);
    on<LogoutEvent>(_onLogout);
  }

  Future<void> _onCheckAuth(CheckAuthEvent event, Emitter<AuthState> emit) async {
    emit(AuthLoadingState());
    try {
      final session = await authRepository.getSavedSession();
      if (session != null) {
        emit(AuthenticatedState(session));
      } else {
        emit(UnauthenticatedState());
      }
    } catch (_) {
      emit(UnauthenticatedState());
    }
  }

  Future<void> _onLoginSubmitted(LoginSubmittedEvent event, Emitter<AuthState> emit) async {
    emit(AuthLoadingState());
    try {
      final session = await authRepository.login(
        email: event.email,
        password: event.password,
      );
      emit(AuthenticatedState(session));
    } catch (e) {
      emit(AuthFailureState(e.toString().replaceAll('Exception: ', '')));
      emit(UnauthenticatedState());
    }
  }

  Future<void> _onRegisterSubmitted(RegisterSubmittedEvent event, Emitter<AuthState> emit) async {
    emit(AuthLoadingState());
    try {
      await authRepository.register(
        email: event.email,
        username: event.username,
        password: event.password,
        givenName: event.givenName,
        familyName: event.familyName,
      );
      emit(const RegisterSuccessState('Registration successful! Please login.'));
      emit(UnauthenticatedState());
    } catch (e) {
      emit(AuthFailureState(e.toString().replaceAll('Exception: ', '')));
      emit(UnauthenticatedState());
    }
  }

  Future<void> _onLogout(LogoutEvent event, Emitter<AuthState> emit) async {
    emit(AuthLoadingState());
    await authRepository.logout();
    emit(UnauthenticatedState());
  }
}
