// Runs on a device or emulator (flutter test integration_test), against the in-memory fake API: never against
// production. See test/support/journeys.dart for the journey itself.
import 'package:flutter_test/flutter_test.dart';
import 'package:integration_test/integration_test.dart';

import '../test/support/journeys.dart';

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  testWidgets('sign in, play, lose an answer, recover it, top up', (t) async {
    await playJourney(t, size: t.view.physicalSize / t.view.devicePixelRatio);
  });
}
