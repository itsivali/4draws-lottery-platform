import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../app/theme/tokens.dart';
import '../../../core/api/api_failure.dart';
import '../../../core/api/idempotency.dart';
import '../../../core/widgets/auth_page.dart';
import '../../../core/widgets/failure_text.dart';
import '../../../core/widgets/inline_banner.dart';
import '../../../core/widgets/status_pill.dart';
import '../../account/application/account_providers.dart';
import '../application/auth_providers.dart';

/// The page an emailed link opens (`/verify-email?token=…`, ADR 0040): it confirms the address at once, signed in or
/// not, and says how it went. The link is often opened on another device than the one the account is open on.
class VerifyEmailScreen extends ConsumerStatefulWidget {
  const VerifyEmailScreen({super.key, required this.token});

  final String token;

  @override
  ConsumerState<VerifyEmailScreen> createState() => _VerifyEmailScreenState();
}

enum _Outcome { working, confirmed, stale, failed }

class _VerifyEmailScreenState extends ConsumerState<VerifyEmailScreen> {
  var _outcome = _Outcome.working;
  ({String title, String? detail})? _error;

  @override
  void initState() {
    super.initState();
    if (widget.token.isEmpty) {
      _outcome = _Outcome.stale;
    } else {
      WidgetsBinding.instance.addPostFrameCallback((_) => _confirm());
    }
  }

  Future<void> _confirm() async {
    setState(() {
      _outcome = _Outcome.working;
      _error = null;
    });
    try {
      await ref.read(authRepositoryProvider).verifyEmail(token: widget.token, idempotencyKey: newIdempotencyKey());
      ref.invalidate(accountProvider); // an open account now shows the address as confirmed
      if (mounted) setState(() => _outcome = _Outcome.confirmed);
    } on ApiFailure catch (f) {
      if (!mounted) return;
      setState(() {
        if (f case ProblemFailure(slug: 'invalid-token')) {
          _outcome = _Outcome.stale;
        } else {
          _outcome = _Outcome.failed;
          _error = describeFailure(f);
        }
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    final (title, lead) = switch (_outcome) {
      _Outcome.working => ('Confirming your email address', 'One moment.'),
      _Outcome.confirmed => ('Your email address is confirmed', 'Thank you. There is nothing more to do.'),
      _Outcome.stale => (
        'This link no longer works',
        'Links work for 24 hours and once. If your address already shows as confirmed in the app, there is nothing '
            'more to do. Otherwise ask for a new link under Account, Email address.',
      ),
      _Outcome.failed => ('We could not confirm it just now', 'Nothing was changed. Try again.'),
    };
    return AuthPage(
      title: title,
      lead: lead,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          switch (_outcome) {
            _Outcome.working => const Center(
              child: Padding(
                padding: EdgeInsets.symmetric(vertical: Space.lg),
                child: CircularProgressIndicator(semanticsLabel: 'Confirming your email address'),
              ),
            ),
            _Outcome.confirmed => const InlineBanner(
              tone: StatusTone.success,
              icon: Icons.mark_email_read_rounded,
              live: true,
              title: 'Confirmed',
              message:
                  'Password-reset links and copies of your data go to this address. If it is the address of your '
                  'Google account, you can sign in with Google too.',
            ),
            _Outcome.stale => const SizedBox.shrink(),
            _Outcome.failed => InlineBanner(
              tone: StatusTone.error,
              live: true,
              title: _error?.title ?? 'Something went wrong',
              message: _error?.detail,
            ),
          },
          if (_outcome != _Outcome.working) ...[
            const SizedBox(height: Space.xl),
            if (_outcome == _Outcome.failed) ...[
              FilledButton(onPressed: _confirm, child: const Text('Try again')),
              const SizedBox(height: Space.sm),
              TextButton(onPressed: () => context.go('/home'), child: const Text('Open 4draws')),
            ] else
              // signed out, this lands on sign-in; signed in, on Home
              FilledButton(onPressed: () => context.go('/home'), child: const Text('Open 4draws')),
          ],
        ],
      ),
    );
  }
}
