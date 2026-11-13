import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../app/providers.dart';
import '../../../app/theme/tokens.dart';
import '../../../core/api/api_failure.dart';
import '../../../core/api/idempotency.dart';
import '../../../core/auth/session_manager.dart';
import '../../../core/widgets/auth_page.dart';
import '../../../core/widgets/busy_button.dart';
import '../../../core/widgets/failure_text.dart';
import '../../../core/widgets/inline_banner.dart';
import '../../../core/widgets/status_pill.dart';
import '../application/auth_providers.dart';
import 'email_rules.dart';

/// The signed-out frame the reset screens share with sign-in: the form's card in the middle of the brand's colour.
class _SignedOutPage extends StatelessWidget {
  const _SignedOutPage({required this.title, required this.lead, required this.children});

  final String title;
  final String lead;
  final List<Widget> children;

  @override
  Widget build(BuildContext context) => AuthPage(
    title: title,
    lead: lead,
    child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: children),
  );
}

/// "Forgotten your password?": the player gives the email address of their account and is emailed a link (ADR 0021).
/// The answer is the same whether or not the address has an account.
class ForgotPasswordScreen extends ConsumerStatefulWidget {
  const ForgotPasswordScreen({super.key});

  @override
  ConsumerState<ForgotPasswordScreen> createState() => _ForgotPasswordScreenState();
}

class _ForgotPasswordScreenState extends ConsumerState<ForgotPasswordScreen> {
  final _form = GlobalKey<FormState>();
  final _email = TextEditingController();
  var _busy = false;
  var _sent = false;
  String? _error, _errorDetail;

  @override
  void dispose() {
    _email.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    setState(() => _error = _errorDetail = null);
    if (!_form.currentState!.validate()) return;
    setState(() => _busy = true);
    try {
      await ref
          .read(authRepositoryProvider)
          .forgotPassword(email: _email.text.trim(), idempotencyKey: newIdempotencyKey());
      if (mounted) setState(() => _sent = true);
    } on ApiFailure catch (f) {
      if (!mounted) return;
      final d = describeFailure(f);
      setState(() {
        _error = d.title;
        _errorDetail = d.detail;
      });
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) => _SignedOutPage(
    title: 'Reset your password',
    lead: _sent
        ? 'Check your inbox.'
        : 'Give the email address of your account and we will email you a link to choose a new password.',
    children: [
      if (_sent) ...[
        const InlineBanner(
          tone: StatusTone.success,
          icon: Icons.mark_email_read_rounded,
          live: true,
          title: 'If that address has an account, a link is on its way.',
          message: 'It works for 30 minutes. Not there? Check the spam folder, or ask again.',
        ),
        const SizedBox(height: Space.xl),
        FilledButton(onPressed: () => context.go('/sign-in'), child: const Text('Back to sign in')),
      ] else ...[
        Form(
          key: _form,
          child: TextFormField(
            controller: _email,
            keyboardType: TextInputType.emailAddress,
            autofillHints: const [AutofillHints.email],
            autocorrect: false,
            textInputAction: TextInputAction.done,
            onFieldSubmitted: (_) => _submit(),
            decoration: const InputDecoration(
              labelText: 'Email address',
              prefixIcon: Icon(Icons.alternate_email_rounded),
            ),
            validator: emailFieldError,
          ),
        ),
        if (_error != null) ...[
          const SizedBox(height: Space.lg),
          InlineBanner(tone: StatusTone.error, live: true, title: _error!, message: _errorDetail),
        ],
        const SizedBox(height: Space.xl),
        BusyButton(label: 'Email me a link', busy: _busy, onPressed: _submit),
        const SizedBox(height: Space.sm),
        TextButton(onPressed: _busy ? null : () => context.go('/sign-in'), child: const Text('Back to sign in')),
      ],
    ],
  );
}

/// The page an emailed link opens (`/reset-password?token=…`): a new password, then back to sign in.
class ResetPasswordScreen extends ConsumerStatefulWidget {
  const ResetPasswordScreen({super.key, required this.token});

  final String token;

  @override
  ConsumerState<ResetPasswordScreen> createState() => _ResetPasswordScreenState();
}

class _ResetPasswordScreenState extends ConsumerState<ResetPasswordScreen> {
  final _form = GlobalKey<FormState>();
  final _password = TextEditingController();
  var _busy = false;
  var _obscure = true;
  var _expired = false;
  String? _passwordError, _error, _errorDetail;

  @override
  void dispose() {
    _password.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    setState(() => _passwordError = _error = _errorDetail = null);
    if (!_form.currentState!.validate()) return;
    setState(() => _busy = true);
    try {
      await ref
          .read(authRepositoryProvider)
          .resetPassword(token: widget.token, password: _password.text, idempotencyKey: newIdempotencyKey());
      // the reset ended every session, this device's too if it was signed in: forget it here, then say so on sign-in
      final session = ref.read(sessionManagerProvider);
      if (session.status.value == SessionStatus.signedIn) await session.endedByServer(SessionEndReason.signedOut);
      if (!mounted) return;
      context.go('/sign-in?reset=1');
    } on ApiFailure catch (f) {
      if (!mounted) return;
      setState(() {
        switch (f) {
          case ProblemFailure(slug: 'weak-password', :final title, :final detail):
            _passwordError = detail == null ? title : '$title: $detail';
          case ProblemFailure(slug: 'invalid-token'):
            _expired = true;
          default:
            final d = describeFailure(f);
            _error = d.title;
            _errorDetail = d.detail;
        }
      });
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    if (widget.token.isEmpty || _expired) {
      return _SignedOutPage(
        title: 'This link no longer works',
        lead: widget.token.isEmpty
            ? 'The link is incomplete. Open the one in the email, or ask for a new one.'
            : 'Links work for 30 minutes and once. Ask for a new one and we will email it to you.',
        children: [
          FilledButton(onPressed: () => context.go('/forgot-password'), child: const Text('Ask for a new link')),
          const SizedBox(height: Space.sm),
          TextButton(onPressed: () => context.go('/sign-in'), child: const Text('Back to sign in')),
        ],
      );
    }
    return _SignedOutPage(
      title: 'Choose a new password',
      lead: 'Every device signed in to your account is signed out when you save it.',
      children: [
        Form(
          key: _form,
          child: AutofillGroup(
            child: TextFormField(
              controller: _password,
              obscureText: _obscure,
              autofillHints: const [AutofillHints.newPassword],
              textInputAction: TextInputAction.done,
              onFieldSubmitted: (_) => _submit(),
              decoration: InputDecoration(
                labelText: 'New password',
                helperText: '8 to 128 characters, not a common password, not your number',
                errorText: _passwordError,
                prefixIcon: const Icon(Icons.lock_rounded),
                suffixIcon: IconButton(
                  tooltip: _obscure ? 'Show password' : 'Hide password',
                  icon: Icon(_obscure ? Icons.visibility_rounded : Icons.visibility_off_rounded),
                  onPressed: () => setState(() => _obscure = !_obscure),
                ),
              ),
              validator: (v) => (v ?? '').length < 8 ? 'Use at least 8 characters.' : null,
            ),
          ),
        ),
        if (_error != null) ...[
          const SizedBox(height: Space.lg),
          InlineBanner(tone: StatusTone.error, live: true, title: _error!, message: _errorDetail),
        ],
        const SizedBox(height: Space.xl),
        BusyButton(label: 'Save new password', busy: _busy, onPressed: _submit),
      ],
    );
  }
}
