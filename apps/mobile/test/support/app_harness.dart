import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:lottery_mobile/app/app.dart';
import 'package:lottery_mobile/app/config.dart';
import 'package:lottery_mobile/app/providers.dart';
import 'package:lottery_mobile/core/auth/session_manager.dart';
import 'package:lottery_mobile/core/auth/token_store.dart';
import 'package:lottery_mobile/core/platform/haptics.dart';
import 'package:lottery_mobile/core/platform/preferences.dart';
import 'package:lottery_mobile/core/widgets/policy_pages.dart';
import 'package:lottery_mobile/features/auth/application/auth_providers.dart';
import 'package:lottery_mobile/features/auth/data/provider_sign_in.dart';
import 'package:lottery_mobile/features/auth/data/sign_in_providers.dart';
import 'package:lottery_mobile/features/play/application/pending_play_store.dart';

import 'fake_backend.dart';

/// Stands in for Google's and Apple's own sign-in: every tap yields [nextToken] at once.
class FakeProviderSignIn implements ProviderSignIn {
  final _tokens = StreamController<ProviderToken>.broadcast();
  SignInProviders? prepared;
  int opened = 0;
  String nextToken = 'google-id-token';

  @override
  Future<void> prepare(SignInProviders providers) async => prepared = providers;
  @override
  Widget? googleButton() => null;
  @override
  Stream<ProviderToken> get tokens => _tokens.stream;
  @override
  Future<void> google() async {
    opened++;
    _tokens.add(ProviderToken(provider: 'google', idToken: nextToken, nonce: 'n1'));
  }

  @override
  Future<void> apple(SignInProviders providers) async {
    opened++;
    _tokens.add(ProviderToken(provider: 'apple', idToken: nextToken, nonce: 'n1'));
  }
}

class RecordingHaptics implements Haptics {
  final events = <HapticEvent>[];
  @override
  void play(HapticEvent e) => events.add(e);
}

class AppHarness {
  AppHarness(this.container, this.backend, this.haptics, this.pending, this.providerSignIn);

  final ProviderContainer container;
  final FakeBackend backend;
  final RecordingHaptics haptics;
  final MemoryPendingPlayStore pending;
  final FakeProviderSignIn providerSignIn;
}

/// Runs the whole app against [backend] at a given screen size and accessibility settings.
Future<AppHarness> pumpApp(
  WidgetTester tester,
  FakeBackend backend, {
  Size size = const Size(390, 844),
  double textScale = 1,
  bool reduceMotion = false,
  bool signedIn = true,
  bool hapticsOn = true,
  String? storedRefresh, // a refresh token kept from an earlier run (signedIn: false restores it)
}) async {
  tester.view.physicalSize = size * 2;
  tester.view.devicePixelRatio = 2;
  tester.platformDispatcher.textScaleFactorTestValue = textScale;
  tester.platformDispatcher.accessibilityFeaturesTestValue = FakeAccessibilityFeatures(disableAnimations: reduceMotion);
  addTearDown(tester.view.reset);
  addTearDown(tester.platformDispatcher.clearAllTestValues);

  final haptics = RecordingHaptics();
  final prefs = MemoryPreferenceStore()..values['wheel_haptics'] = hapticsOn;
  final pending = MemoryPendingPlayStore();
  final providerSignIn = FakeProviderSignIn();
  late final ProviderContainer container;
  container = ProviderContainer(
    overrides: [
      appConfigProvider.overrideWithValue(AppConfig(apiBaseUrl: Uri.parse('https://api.test'))),
      httpClientProvider.overrideWithValue(backend.client),
      tokenStoreProvider.overrideWithValue(MemoryTokenStore()..value = storedRefresh),
      deviceIdentityProvider.overrideWithValue(const DeviceIdentity(platform: 'android', fingerprint: 'fp-test')),
      pendingPlayStoreProvider.overrideWithValue(pending),
      providerSignInProvider.overrideWithValue(providerSignIn),
      preferenceStoreProvider.overrideWithValue(prefs),
      storedHapticsPreferenceProvider.overrideWithValue(hapticsOn),
      hapticsSupportedProvider.overrideWithValue(true),
      urlOpenerProvider.overrideWithValue((url) async {
        backend.opened.add(url);
        return true;
      }),
      hapticsProvider.overrideWith((ref) => GatedHaptics(haptics, enabled: () => ref.read(hapticsEnabledProvider))),
    ],
    retry: (_, _) => null,
  );
  addTearDown(container.dispose);
  final session = container.read(sessionManagerProvider);
  if (signedIn) {
    await session.signedIn(
      Tokens(access: backend.access, refresh: 'refresh-1', expiresAt: DateTime.now().add(const Duration(hours: 1))),
    );
  } else {
    await session.restore();
  }
  await tester.pumpWidget(UncontrolledProviderScope(container: container, child: const LotteryApp()));
  await tester.pumpAndSettle();
  return AppHarness(container, backend, haptics, pending, providerSignIn);
}

/// Pumps frames until [finder] appears (the wheel's spin takes a few seconds of fake time).
Future<void> pumpUntilFound(
  WidgetTester tester,
  Finder finder, {
  Duration step = const Duration(milliseconds: 50),
  int max = 400,
}) async {
  for (var i = 0; i < max; i++) {
    if (finder.evaluate().isNotEmpty) return;
    await tester.pump(step);
  }
  expect(finder, findsWidgets);
}
