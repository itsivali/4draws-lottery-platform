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
