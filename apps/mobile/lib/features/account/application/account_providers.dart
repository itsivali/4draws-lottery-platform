import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../app/providers.dart';
import '../data/account_repository.dart';
import '../domain/account_models.dart';

final accountRepositoryProvider = Provider<AccountRepository>((ref) => AccountRepository(ref.watch(apiClientProvider)));

final accountProvider = FutureProvider<Account>((ref) async {
  final a = await ref.watch(accountRepositoryProvider).account();
  // a temporary password set by staff is remembered across restarts: the account says so until it is replaced
  ref.read(sessionManagerProvider).mustChangePassword.value = a.mustChangePassword;
  return a;
});

final limitsProvider = FutureProvider<Map<String, LimitState>>((ref) => ref.watch(accountRepositoryProvider).limits());

/// The names and ID number the player gave at sign-up (null for accounts from before 2026-10-30).
final declaredIdentityProvider = FutureProvider<DeclaredIdentity?>(
  (ref) => ref.watch(accountRepositoryProvider).declaredIdentity(),
);

/// The devices signed in to the account.
final sessionsProvider = FutureProvider<List<DeviceSession>>((ref) => ref.watch(accountRepositoryProvider).sessions());

/// Whether the account has a password and which Google or Apple identities sign in to it.
final signInsProvider = FutureProvider<SignInMethods>((ref) => ref.watch(accountRepositoryProvider).signIns());
