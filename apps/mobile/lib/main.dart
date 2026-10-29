import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_web_plugins/url_strategy.dart';

import 'app/app.dart';
import 'app/config.dart';
import 'app/providers.dart';
import 'core/platform/device_identity.dart';
import 'core/platform/preferences.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  usePathUrlStrategy(); // /sign-in, not /#/sign-in: crawlable, shareable addresses (docs/runbooks/web-seo.md)
  final config = AppConfig.fromEnvironment();
  if (config == null) {
    runApp(const ConfigErrorApp());
    return;
  }
  final device = await loadDeviceIdentity();
  final haptics = await loadStoredHapticsPreference(SharedPreferenceStore());
  final container = ProviderContainer(
    overrides: [
      appConfigProvider.overrideWithValue(config),
      deviceIdentityProvider.overrideWithValue(device),
      storedHapticsPreferenceProvider.overrideWithValue(haptics),
    ],
    // no silent automatic retries: reads offer "Try again", and money or play actions reconcile explicitly
    retry: (_, _) => null,
  );
  container.read(sessionManagerProvider).restore().ignore();
  runApp(UncontrolledProviderScope(container: container, child: const LotteryApp()));
}
