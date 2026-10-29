import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../core/auth/session_manager.dart';
import '../core/widgets/inline_banner.dart';
import '../features/account/presentation/account_screen.dart';
import '../features/account/presentation/closure_screen.dart';
import '../features/account/presentation/email_screen.dart';
import '../features/account/presentation/how_it_works_screen.dart';
import '../features/account/presentation/kyc_screen.dart';
import '../features/account/presentation/name_screen.dart';
import '../features/account/presentation/password_screen.dart';
import '../features/account/presentation/responsible_gaming_screen.dart';
import '../features/auth/presentation/password_reset_screens.dart';
import '../features/auth/presentation/session_screens.dart';
import '../features/auth/presentation/sign_in_screen.dart';
import '../features/draws/presentation/draw_detail_screen.dart';
import '../features/draws/presentation/draws_screen.dart';
import '../features/fairness/presentation/fairness_screen.dart';
import '../features/home/presentation/home_screen.dart';
import '../features/play/presentation/play_screen.dart';
import '../features/wallet/presentation/top_up_screen.dart';
import '../features/wallet/presentation/wallet_screen.dart';
import '../features/wallet/presentation/withdraw_screen.dart';
import 'providers.dart';
import 'shell.dart';

const _sessionRoutes = {'/sign-in', '/forgot-password', '/reset-password', '/starting', '/offline'};

/// Where a signed-out player may be besides sign-in: asking for, and using, a password-reset link (ADR 0021).
const _signedOutRoutes = {'/sign-in', '/forgot-password', '/reset-password'};

/// Where the session says the player may be: nowhere but sign-in while signed out; never on sign-in while signed in.
/// Deep links survive the detour through sign-in (`from`).
/// The password screen a player with a temporary password is held on until they choose their own.
const requiredPasswordRoute = '/account/password?required=1';

String? sessionRedirect(SessionStatus status, Uri location, {bool mustChangePassword = false}) {
  final path = location.path;
  switch (status) {
    case SessionStatus.restoring:
      return path == '/starting' ? null : '/starting?from=${Uri.encodeComponent(location.toString())}';
    case SessionStatus.offline:
      return path == '/offline' ? null : '/offline?from=${Uri.encodeComponent(location.toString())}';
    case SessionStatus.signedOut:
      if (_signedOutRoutes.contains(path)) return null;
      final from = _sessionRoutes.contains(path) ? location.queryParameters['from'] : location.toString();
      return from == null || from == '/' ? '/sign-in' : '/sign-in?from=${Uri.encodeComponent(from)}';
    case SessionStatus.signedIn:
      if (mustChangePassword) return path == '/account/password' ? null : requiredPasswordRoute;
      if (!_sessionRoutes.contains(path)) return path == '/' ? '/home' : null;
      final from = location.queryParameters['from'];
      return from == null || from.isEmpty || from == '/' || _sessionRoutes.contains(Uri.parse(from).path)
          ? '/home'
          : from;
  }
}

final routerProvider = Provider<GoRouter>((ref) {
  final session = ref.watch(sessionManagerProvider);
  final router = GoRouter(
    initialLocation: '/home',
    refreshListenable: Listenable.merge([session.status, session.mustChangePassword]),
    redirect: (context, state) =>
        sessionRedirect(session.status.value, state.uri, mustChangePassword: session.mustChangePassword.value),
    routes: [
      GoRoute(path: '/', redirect: (_, _) => '/home'),
      GoRoute(path: '/starting', builder: (_, _) => const StartingScreen()),
      GoRoute(path: '/offline', builder: (_, _) => const OfflineScreen()),
      GoRoute(path: '/sign-in', builder: (_, _) => const SignInScreen()),
      GoRoute(path: '/forgot-password', builder: (_, _) => const ForgotPasswordScreen()),
      GoRoute(
        path: '/reset-password',
        builder: (_, state) => ResetPasswordScreen(token: state.uri.queryParameters['token'] ?? ''),
      ),
      StatefulShellRoute.indexedStack(
        builder: (_, _, shell) => AppShell(shell: shell),
        branches: [
          StatefulShellBranch(
            routes: [GoRoute(path: '/home', builder: (_, _) => const HomeScreen())],
          ),
          StatefulShellBranch(
            routes: [GoRoute(path: '/play', builder: (_, _) => const PlayScreen())],
          ),
          StatefulShellBranch(
            routes: [
              GoRoute(
                path: '/draws',
                builder: (_, _) => const DrawsScreen(),
                routes: [
                  GoRoute(
                    path: ':id',
                    builder: (_, s) => DrawDetailScreen(id: s.pathParameters['id']!),
                  ),
                ],
              ),
            ],
          ),
          StatefulShellBranch(
            routes: [
              GoRoute(
                path: '/wallet',
                builder: (_, _) => const WalletScreen(),
                routes: [
                  GoRoute(path: 'top-up', builder: (_, _) => const TopUpScreen()),
                  GoRoute(path: 'withdraw', builder: (_, _) => const WithdrawScreen()),
                ],
              ),
            ],
          ),
          StatefulShellBranch(
            routes: [
              GoRoute(
                path: '/account',
                builder: (_, _) => const AccountScreen(),
                routes: [
                  GoRoute(path: 'verify', builder: (_, _) => const KycScreen()),
                  GoRoute(path: 'limits', builder: (_, _) => const ResponsibleGamingScreen()),
                  GoRoute(path: 'password', builder: (_, _) => const PasswordScreen()),
                  GoRoute(path: 'email', builder: (_, _) => const EmailScreen()),
                  GoRoute(path: 'name', builder: (_, _) => const NameScreen()),
                  GoRoute(path: 'fairness', builder: (_, _) => const FairnessScreen()),
                  GoRoute(path: 'how-it-works', builder: (_, _) => const HowItWorksScreen()),
                  GoRoute(path: 'close', builder: (_, _) => const ClosureScreen()),
                ],
              ),
            ],
          ),
        ],
      ),
    ],
    errorBuilder: (context, state) => Scaffold(
      appBar: AppBar(title: const Text('Not found')),
      body: Center(
        child: EmptyState(
          icon: Icons.explore_off_rounded,
          title: 'That page does not exist',
          message: 'The link may be old or mistyped.',
          actionLabel: 'Go to Home',
          onAction: () => context.go('/home'),
        ),
      ),
    ),
  );
  ref.onDispose(router.dispose);
  return router;
});
