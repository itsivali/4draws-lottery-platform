import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../core/auth/session_manager.dart';
import '../core/widgets/inline_banner.dart';
import '../features/account/presentation/account_screen.dart';
import '../features/account/presentation/closure_screen.dart';
import '../features/account/presentation/data_copy_screen.dart';
import '../features/account/presentation/details_screen.dart';
import '../features/account/presentation/devices_screen.dart';
import '../features/account/presentation/email_screen.dart';
import '../features/account/presentation/how_it_works_screen.dart';
import '../features/account/presentation/kyc_screen.dart';
import '../features/account/presentation/password_screen.dart';
import '../features/account/presentation/responsible_gaming_screen.dart';
import '../features/account/presentation/sign_ins_screen.dart';
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

const _sessionRoutes = {'/sign-in', '/create-account', '/forgot-password', '/reset-password', '/starting', '/offline'};

/// Where a signed-out player may be besides sign-in: asking for, and using, a password-reset link (ADR 0021).
const _signedOutRoutes = {'/sign-in', '/create-account', '/forgot-password', '/reset-password'};

/// Where the session says the player may be: nowhere but sign-in while signed out; never on sign-in while signed in.
/// Deep links survive the detour through sign-in (`from`).
/// The password screen a player with a temporary password is held on until they choose their own.
const requiredPasswordRoute = '/account/password?required=1';

/// Addresses people guess or bookmark for signing in and signing up, sent to the real ones (owner, 2026-11-11: /login
/// ended on "page may not exist").
const _aliases = {
  '/login': '/sign-in',
  '/log-in': '/sign-in',
  '/signin': '/sign-in',
  '/register': '/create-account',
  '/signup': '/create-account',
  '/sign-up': '/create-account',
};

/// The app's own pages, under its five destinations: the only places a detour through sign-in may return to.
const _appAreas = ['/home', '/play', '/draws', '/wallet', '/account'];

bool _isAppPage(String location) {
  final path = Uri.tryParse(location)?.path ?? '';
  return _appAreas.any((a) => path == a || path.startsWith('$a/'));
}

String? sessionRedirect(SessionStatus status, Uri location, {bool mustChangePassword = false}) {
  final path = location.path;
  if (_aliases[path.toLowerCase()] case final real?) return real;
  switch (status) {
    case SessionStatus.restoring:
      return path == '/starting' ? null : '/starting?from=${Uri.encodeComponent(location.toString())}';
    case SessionStatus.offline:
      return path == '/offline' ? null : '/offline?from=${Uri.encodeComponent(location.toString())}';
    case SessionStatus.signedOut:
      if (_signedOutRoutes.contains(path)) return null;
      final from = _sessionRoutes.contains(path) ? location.queryParameters['from'] : location.toString();
      // only a real page is worth coming back to; anything else returns home after sign-in
      return from == null || !_isAppPage(from) ? '/sign-in' : '/sign-in?from=${Uri.encodeComponent(from)}';
    case SessionStatus.signedIn:
      if (mustChangePassword) return path == '/account/password' ? null : requiredPasswordRoute;
      if (!_sessionRoutes.contains(path)) return path == '/' ? '/home' : null;
      final from = location.queryParameters['from'];
      return from != null && _isAppPage(from) ? from : '/home';
  }
}

/// Every screen as a Material page, so each platform gets its own transition from the theme (fade-forwards on
/// Android, the slide and edge swipe on iOS).
GoRouterPageBuilder _page(GoRouterWidgetBuilder build) =>
    (context, state) => MaterialPage<void>(
      key: state.pageKey,
      name: state.name,
      arguments: state.extra,
      restorationId: state.pageKey.value,
      child: build(context, state),
    );

final routerProvider = Provider<GoRouter>((ref) {
  final session = ref.watch(sessionManagerProvider);
  final router = GoRouter(
    initialLocation: '/home',
    refreshListenable: Listenable.merge([session.status, session.mustChangePassword]),
    redirect: (context, state) =>
        sessionRedirect(session.status.value, state.uri, mustChangePassword: session.mustChangePassword.value),
    routes: [
      GoRoute(path: '/', redirect: (_, _) => '/home'),
      GoRoute(path: '/starting', pageBuilder: _page((_, _) => const StartingScreen())),
      GoRoute(path: '/offline', pageBuilder: _page((_, _) => const OfflineScreen())),
      GoRoute(path: '/sign-in', pageBuilder: _page((_, _) => const SignInScreen())),
      // its own address, so search engines can list "Create account" beside "Sign in" (2026-10-30)
      GoRoute(path: '/create-account', pageBuilder: _page((_, _) => const SignInScreen(create: true))),
      GoRoute(path: '/forgot-password', pageBuilder: _page((_, _) => const ForgotPasswordScreen())),
      GoRoute(
        path: '/reset-password',
        pageBuilder: _page((_, state) => ResetPasswordScreen(token: state.uri.queryParameters['token'] ?? '')),
      ),
      StatefulShellRoute.indexedStack(
        builder: (_, _, shell) => AppShell(shell: shell),
        branches: [
          StatefulShellBranch(
            routes: [GoRoute(path: '/home', pageBuilder: _page((_, _) => const HomeScreen()))],
          ),
          StatefulShellBranch(
            routes: [GoRoute(path: '/play', pageBuilder: _page((_, _) => const PlayScreen()))],
          ),
          StatefulShellBranch(
            routes: [
              GoRoute(
                path: '/draws',
                pageBuilder: _page((_, _) => const DrawsScreen()),
                routes: [
                  GoRoute(
                    path: ':id',
                    pageBuilder: _page((_, s) => DrawDetailScreen(id: s.pathParameters['id']!)),
                  ),
                ],
              ),
            ],
          ),
          StatefulShellBranch(
            routes: [
              GoRoute(
                path: '/wallet',
                pageBuilder: _page((_, _) => const WalletScreen()),
                routes: [
                  GoRoute(path: 'top-up', pageBuilder: _page((_, _) => const TopUpScreen())),
                  GoRoute(path: 'withdraw', pageBuilder: _page((_, _) => const WithdrawScreen())),
                ],
              ),
            ],
          ),
          StatefulShellBranch(
            routes: [
              GoRoute(
                path: '/account',
                pageBuilder: _page((_, _) => const AccountScreen()),
                routes: [
                  GoRoute(path: 'details', pageBuilder: _page((_, _) => const DetailsScreen())),
                  GoRoute(path: 'verify', pageBuilder: _page((_, _) => const KycScreen())),
                  GoRoute(path: 'devices', pageBuilder: _page((_, _) => const DevicesScreen())),
                  GoRoute(path: 'sign-ins', pageBuilder: _page((_, _) => const SignInsScreen())),
                  GoRoute(path: 'limits', pageBuilder: _page((_, _) => const ResponsibleGamingScreen())),
                  GoRoute(path: 'password', pageBuilder: _page((_, _) => const PasswordScreen())),
                  GoRoute(path: 'email', pageBuilder: _page((_, _) => const EmailScreen())),
                  GoRoute(path: 'fairness', pageBuilder: _page((_, _) => const FairnessScreen())),
                  GoRoute(path: 'how-it-works', pageBuilder: _page((_, _) => const HowItWorksScreen())),
                  GoRoute(path: 'data', pageBuilder: _page((_, _) => const DataCopyScreen())),
                  GoRoute(path: 'close', pageBuilder: _page((_, _) => const ClosureScreen())),
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
