import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../app/providers.dart';
import '../data/auth_repository.dart';
import '../data/provider_sign_in.dart';
import '../data/sign_in_providers.dart';

final authRepositoryProvider = Provider<AuthRepository>(
  (ref) => AuthRepository(ref.watch(apiClientProvider), ref.watch(deviceIdentityProvider)),
);

/// Which of Google and Apple the API offers. When the API cannot be asked, none: the phone-and-password form is
/// always there, and the buttons appear once it answers.
final signInProvidersProvider = FutureProvider<SignInProviders>((ref) async {
  try {
    return await ref.watch(authRepositoryProvider).providers();
  } on Exception {
    return SignInProviders.none;
  }
});

/// The providers' own sign-in on this platform; a fake in tests.
final providerSignInProvider = Provider<ProviderSignIn>(
  (ref) => SdkProviderSignIn(apiBaseUrl: ref.watch(appConfigProvider).apiBaseUrl),
);
