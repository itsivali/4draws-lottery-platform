import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../app/providers.dart';
import '../../../app/theme/tokens.dart';
import '../../../core/api/api_failure.dart';
import '../../../core/api/idempotency.dart';
import '../../../core/widgets/app_card.dart';
import '../../../core/widgets/busy_button.dart';
import '../../../core/widgets/content_frame.dart';
import '../../../core/widgets/failure_text.dart';
import '../../../core/widgets/inline_banner.dart';
import '../../../core/widgets/status_pill.dart';
import '../../auth/application/auth_providers.dart';
import '../application/account_providers.dart';

class PasswordScreen extends ConsumerStatefulWidget {
  const PasswordScreen({super.key});

  @override
  ConsumerState<PasswordScreen> createState() => _PasswordScreenState();
}

class _PasswordScreenState extends ConsumerState<PasswordScreen> {
  final _form = GlobalKey<FormState>();
  final _current = TextEditingController();
  final _next = TextEditingController();
  var _busy = false;
  var _obscure = true;
  String? _error;
  var _sendingLink = false;
  String? _linkSentTo, _linkError;

  @override
  void dispose() {
    _current.dispose();
    _next.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    setState(() => _error = null);
    if (!_form.currentState!.validate()) return;
    setState(() => _busy = true);
    try {
      final first = _first(context);
      final repo = ref.read(accountRepositoryProvider);
      if (first) {
        await repo.setPassword(next: _next.text, idempotencyKey: newIdempotencyKey());
      } else {
        await repo.changePassword(current: _current.text, next: _next.text, idempotencyKey: newIdempotencyKey());
      }
      TextInput.finishAutofillContext(); // the password manager offers to update the saved password
      ref.read(sessionManagerProvider).passwordChanged();
      ref
        ..invalidate(accountProvider)
        ..invalidate(signInsProvider);
      if (!mounted) return;
      final required = _required(context);
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(
            first
                ? 'Password set. You can also sign in with your phone number and it. Other devices are signed out.'
                : 'Password changed. Other devices are signed out.',
          ),
        ),
      );
      context.go(required ? '/home' : '/account');
    } on ApiFailure catch (f) {
      if (!mounted) return;
      final d = describeFailure(f);
      setState(() => _error = [d.title, ?d.detail].join(': '));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  /// A signed-in player who forgot their current password is emailed the same reset link the sign-in screen offers
  /// (ADR 0021), to the address on their account.
  Future<void> _emailResetLink(String email) async {
    setState(() {
      _linkError = null;
      _sendingLink = true;
    });
    try {
      await ref.read(authRepositoryProvider).forgotPassword(email: email, idempotencyKey: newIdempotencyKey());
      if (mounted) setState(() => _linkSentTo = email);
    } on ApiFailure catch (f) {
      if (!mounted) return;
      final d = describeFailure(f);
      setState(() => _linkError = [d.title, ?d.detail].join(': '));
    } finally {
      if (mounted) setState(() => _sendingLink = false);
    }
  }

  /// Staff gave this player a temporary password: the router sent them here and holds them until it is replaced.
  static bool _required(BuildContext context) => GoRouterState.of(context).uri.queryParameters['required'] == '1';

  /// An account that signs in with Google or Apple only sets its first password: there is no current one (ADR 0035).
  static bool _first(BuildContext context) => GoRouterState.of(context).uri.queryParameters['set'] == '1';

  @override
  Widget build(BuildContext context) => PageScaffold(
    title: _required(context)
        ? 'Choose a new password'
        : _first(context)
        ? 'Set a password'
        : 'Change password',
    children: [
      Form(
        key: _form,
        child: AppCard(
          child: AutofillGroup(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                if (_required(context)) ...[
                  const InlineBanner(
                    tone: StatusTone.pending,
                    icon: Icons.key_rounded,
                    live: true,
                    title: 'You signed in with a temporary password. Choose your own to continue; the temporary one stops working now.',
                  ),
                  const SizedBox(height: Space.lg),
                ],
                if (_first(context)) ...[
                  const InlineBanner(
                    tone: StatusTone.pending,
                    icon: Icons.key_rounded,
                    title: 'With a password you can also sign in with your phone number, on the web too.',
                    message:
                        'For your security this works within 10 minutes of signing in with Google. If it '
                        'is refused, sign out, sign in with Google again, and come straight back here.',
                  ),
                  const SizedBox(height: Space.lg),
                ] else ...[
                  TextFormField(
                    controller: _current,
                    obscureText: _obscure,
                    autofillHints: const [AutofillHints.password],
                    decoration: InputDecoration(
                      labelText: _required(context) ? 'Temporary password' : 'Current password',
                      prefixIcon: const Icon(Icons.lock_outline_rounded),
                      suffixIcon: IconButton(
                        tooltip: _obscure ? 'Show passwords' : 'Hide passwords',
                        icon: Icon(_obscure ? Icons.visibility_rounded : Icons.visibility_off_rounded),
                        onPressed: () => setState(() => _obscure = !_obscure),
                      ),
                    ),
                    validator: (v) => (v ?? '').isEmpty ? 'Enter your current password.' : null,
                  ),
                  const SizedBox(height: Space.lg),
                ],
                TextFormField(
                  controller: _next,
                  obscureText: _obscure,
                  autofillHints: const [AutofillHints.newPassword],
                  decoration: const InputDecoration(
                    labelText: 'New password',
                    prefixIcon: Icon(Icons.lock_rounded),
                    helperText: '8 to 128 characters, not a common password, not your number',
                  ),
                  validator: (v) => (v ?? '').length < 8 ? 'Use at least 8 characters.' : null,
                ),
                if (_error != null) ...[
                  const SizedBox(height: Space.lg),
                  InlineBanner(tone: StatusTone.error, live: true, title: _error!),
                ],
                const SizedBox(height: Space.xl),
                BusyButton(
                  label: _first(context) ? 'Set password' : 'Change password',
                  busy: _busy,
                  onPressed: _submit,
                ),
              ],
            ),
          ),
        ),
      ),
      if (!_first(context) && !_required(context)) ..._forgotten(context),
    ],
  );

  /// "Forgotten your current password?": the reset link, emailed to the address on the account.
  List<Widget> _forgotten(BuildContext context) {
    final email = ref.watch(accountProvider).value?.email;
    return [
      const SizedBox(height: Space.lg),
      AppCard(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Text('Forgotten your current password?', style: Theme.of(context).textTheme.titleMedium),
            const SizedBox(height: Space.xs),
            Text(
              email == null
                  ? 'Add an email address to your account and we can email you a link to choose a new password.'
                  : 'We can email a link to $email. It works for 30 minutes, and using it signs out every device.',
              style: Theme.of(context).textTheme.bodyMedium,
            ),
            if (_linkSentTo != null) ...[
              const SizedBox(height: Space.md),
              InlineBanner(
                tone: StatusTone.success,
                icon: Icons.mark_email_read_rounded,
                live: true,
                title: 'A link is on its way to $_linkSentTo',
                message: 'Not there? Check the junk folder.',
              ),
            ],
            if (_linkError != null) ...[
              const SizedBox(height: Space.md),
              InlineBanner(tone: StatusTone.error, live: true, title: _linkError!),
            ],
            const SizedBox(height: Space.md),
            if (email == null)
              FilledButton.tonal(
                onPressed: () => context.go('/account/email'),
                child: const Text('Add an email address'),
              )
            else
              FilledButton.tonal(
                onPressed: _sendingLink || _linkSentTo != null ? null : () => _emailResetLink(email),
                child: Text(_sendingLink ? 'Sending…' : 'Email me a reset link'),
              ),
          ],
        ),
      ),
    ];
  }
}
