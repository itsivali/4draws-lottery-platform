import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../core/auth/session_manager.dart';
import '../features/account/application/account_providers.dart';
import '../features/draws/application/draws_providers.dart';
import '../features/fairness/application/fairness_providers.dart';
import '../features/play/application/play_controller.dart';
import '../features/play/application/play_providers.dart';
import '../features/wallet/application/money_flows.dart';
import '../features/wallet/application/wallet_providers.dart';
import 'providers.dart';
import 'router.dart';
import 'theme/theme.dart';

class LotteryApp extends ConsumerStatefulWidget {
  const LotteryApp({super.key});

  @override
  ConsumerState<LotteryApp> createState() => _LotteryAppState();
}

class _LotteryAppState extends ConsumerState<LotteryApp> with WidgetsBindingObserver {
  late final SessionManager _session = ref.read(sessionManagerProvider);
  SessionStatus? _last;

  @override
  void initState() {
    super.initState();
    _session.status.addListener(_onSession);
    WidgetsBinding.instance.addObserver(this);
  }

  /// A new session never sees the previous one's data, nor its unfinished play or money request.
  void _onSession() {
    final s = _session.status.value;
    if (s == _last) return;
    _last = s;
    if (s == SessionStatus.signedIn || s == SessionStatus.signedOut) {
      ref
        ..invalidate(accountProvider)
        ..invalidate(currentUserIdProvider)
        ..invalidate(todayProvider)
        ..invalidate(walletStatementProvider)
        ..invalidate(topUpsProvider)
        ..invalidate(payoutsProvider)
        ..invalidate(limitsProvider)
        ..invalidate(fairnessProvider)
        ..invalidate(myEntriesProvider)
        ..invalidate(myPeriodsProvider)
        ..invalidate(drawsProvider)
        ..invalidate(drawProvider)
        ..invalidate(playControllerProvider)
        ..invalidate(topUpFlowProvider)
        ..invalidate(withdrawFlowProvider);
    }
  }

  /// Back from the background (an M-Pesa prompt, a phone call): what the server may have settled meanwhile is
  /// re-read, and a money request still being followed asks again now rather than when its paused timer fires.
  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state != AppLifecycleState.resumed || _session.status.value != SessionStatus.signedIn) return;
    ref
      ..invalidate(walletStatementProvider)
      ..invalidate(topUpsProvider)
      ..invalidate(payoutsProvider)
      ..invalidate(accountProvider)
      ..invalidate(todayProvider);
    if (ref.exists(topUpFlowProvider)) ref.read(topUpFlowProvider).flow.resume();
    if (ref.exists(withdrawFlowProvider)) ref.read(withdrawFlowProvider).flow.resume();
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    _session.status.removeListener(_onSession);
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => MaterialApp.router(
    title: 'Bahati',
    debugShowCheckedModeBanner: false,
    theme: buildTheme(Brightness.light),
    darkTheme: buildTheme(Brightness.dark),
    routerConfig: ref.watch(routerProvider),
  );
}

/// Shown when the build was made without a usable API_BASE_URL: fail closed rather than guess a server.
class ConfigErrorApp extends StatelessWidget {
  const ConfigErrorApp({super.key});

  @override
  Widget build(BuildContext context) => MaterialApp(
    theme: buildTheme(Brightness.light),
    home: const Scaffold(
      body: Center(
        child: Padding(
          padding: EdgeInsets.all(24),
          child: Text(
            'This build has no server configured. Build it with --dart-define=API_BASE_URL=https://… (http only for localhost).',
            textAlign: TextAlign.center,
          ),
        ),
      ),
    ),
  );
}
