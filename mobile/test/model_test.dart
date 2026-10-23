import 'package:flutter_test/flutter_test.dart';
import 'package:social_network_mobile/features/auth/data/models/user_session_model.dart';
import 'package:social_network_mobile/features/feed/data/models/author_model.dart';

void main() {
  group('Models Unit Tests', () {
    test('UserSessionModel parses json correctly', () {
      final json = {
        'id': 'test-uuid-1',
        'email': 'obama@example.com',
        'username': 'obama',
        'role': 'USER',
        'token': 'jwt-sample-token',
      };

      final session = UserSessionModel.fromJson(json);

      expect(session.userId, 'test-uuid-1');
      expect(session.email, 'obama@example.com');
      expect(session.username, 'obama');
      expect(session.role, 'USER');
      expect(session.token, 'jwt-sample-token');
    });

    test('AuthorModel formats full name correctly', () {
      final author = AuthorModel(
        id: 'user-1',
        username: 'obama',
        givenName: 'Barack',
        familyName: 'Obama',
      );

      expect(author.fullName, 'Barack Obama');
    });
  });
}
