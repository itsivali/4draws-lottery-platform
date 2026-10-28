import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:lottery_mobile/app/config.dart';
import 'package:lottery_mobile/app/providers.dart';
import 'package:lottery_mobile/app/theme/theme.dart';
import 'package:lottery_mobile/core/widgets/back_office_link.dart';

void main() {
  Widget app(AppConfig config, {required bool web}) => ProviderScope(
    overrides: [appConfigProvider.overrideWithValue(config)],
    child: MaterialApp(
      theme: buildTheme(),
      home: Scaffold(body: BackOfficeLink(web: web)),
    ),
  );
  final api = Uri.parse('https://api.test');

  testWidgets('the web app shows the back office link when the build names one', (t) async {
    await t.pumpWidget(app(AppConfig(apiBaseUrl: api, backOfficeUrl: Uri.parse('https://office.test')), web: true));
    expect(find.text('Staff? Open the back office'), findsOneWidget);
  });

  testWidgets('the mobile apps, and a web build without an address, show nothing', (t) async {
    await t.pumpWidget(app(AppConfig(apiBaseUrl: api, backOfficeUrl: Uri.parse('https://office.test')), web: false));
    expect(find.text('Staff? Open the back office'), findsNothing);
    await t.pumpWidget(app(AppConfig(apiBaseUrl: api), web: true));
    expect(find.text('Staff? Open the back office'), findsNothing);
  });

  test('only https (or http on a local host) back-office addresses are kept', () {
    expect(AppConfig.secureUrl('https://office.example'), isNotNull);
    expect(AppConfig.secureUrl('http://localhost:5173'), isNotNull);
    expect(AppConfig.secureUrl('http://office.example'), isNull);
    expect(AppConfig.secureUrl('not a url'), isNull);
    expect(AppConfig.secureUrl(''), isNull);
  });
}
