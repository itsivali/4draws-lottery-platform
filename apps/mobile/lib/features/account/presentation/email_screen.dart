import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../app/theme/tokens.dart';
import '../../../core/api/api_failure.dart';
import '../../../core/api/idempotency.dart';
import '../../../core/widgets/app_card.dart';
import '../../../core/widgets/busy_button.dart';
import '../../../core/widgets/content_frame.dart';
import '../../../core/widgets/failure_text.dart';
import '../../../core/widgets/inline_banner.dart';
import '../../../core/widgets/status_pill.dart';
import '../../auth/presentation/email_rules.dart';
import '../application/account_providers.dart';

/// Change, or add, the email address a forgotten password's reset link goes to (ADR 0021). An account with a password
/// gives it; one that signs in with Google or Apple only has none to give.
class EmailScreen extends ConsumerStatefulWidget {
  const EmailScreen({super.key});

  @override
  ConsumerState<EmailScreen> createState() => _EmailScreenState();
}

class _EmailScreenState extends ConsumerState<EmailScreen> {
  final _form = GlobalKey<FormState>();
  final _email = TextEditingController();
  final _password = TextEditingController();
  var _busy = false;
  var _obscure = true;
  var _prefilled = false;
  String? _emailError, _passwordError, _error;

  @override
  void dispose() {
    _email.dispose();
    _password.dispose();
    super.dispose();
  }

  Future<void> _submit({required bool hasPassword}) async {
    setState(() => _emailError = _passwordError = _error = null);
    if (!_form.currentState!.validate()) return;
    setState(() => _busy = true);
    try {
      await ref
          .read(accountRepositoryProvider)
          .setEmail(email: _email.text.trim(), currentPassword: _password.text, idempotencyKey: newIdempotencyKey());
      ref.invalidate(accountProvider);
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('Email address saved.')));
      context.go('/account');
    } on ApiFailure catch (f) {
      if (!mounted) return;
      setState(() {
        switch (f) {
          case ProblemFailure(slug: 'invalid-email' || 'email-registered', :final title, :final detail):
            _emailError = detail == null ? title : '$title: $detail';
          case ProblemFailure(slug: 'wrong-password', :final title):
            _passwordError = title;
          default:
            final d = describeFailure(f);
            _error = [d.title, ?d.detail].join(': ');
        }
      });
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final account = ref.watch(accountProvider).value;
    if (!_prefilled && account != null) {
      _prefilled = true;
      _email.text = account.email ?? '';
    }
    final hasPassword = account?.hasPassword ?? true;
    final adding = account != null && account.email == null;
    return PageScaffold(
      title: adding ? 'Add an email address' : 'Email address',
      children: [
        Form(
          key: _form,
          child: AppCard(
            child: AutofillGroup(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  Text(
                    hasPassword
                        ? 'If you forget your password, the link to choose a new one is emailed here.'
                        : 'You sign in with Google or Apple; the address is kept in case you ever need a password.',
                    style: Theme.of(context).textTheme.bodyMedium,
                  ),
                  const SizedBox(height: Space.lg),
                  TextFormField(
                    controller: _email,
                    keyboardType: TextInputType.emailAddress,
                    autofillHints: const [AutofillHints.email],
                    autocorrect: false,
                    decoration: InputDecoration(
                      labelText: 'Email address',
                      errorText: _emailError,
                      prefixIcon: const Icon(Icons.alternate_email_rounded),
                    ),
                    validator: emailFieldError,
                  ),
                  if (hasPassword) ...[
                    const SizedBox(height: Space.lg),
                    TextFormField(
                      controller: _password,
                      obscureText: _obscure,
                      autofillHints: const [AutofillHints.password],
                      decoration: InputDecoration(
                        labelText: 'Your password',
                        helperText: 'To confirm it is you',
                        errorText: _passwordError,
                        prefixIcon: const Icon(Icons.lock_outline_rounded),
                        suffixIcon: IconButton(
                          tooltip: _obscure ? 'Show password' : 'Hide password',
                          icon: Icon(_obscure ? Icons.visibility_rounded : Icons.visibility_off_rounded),
                          onPressed: () => setState(() => _obscure = !_obscure),
                        ),
                      ),
                      validator: (v) => (v ?? '').isEmpty ? 'Enter your password.' : null,
                    ),
                  ],
                  if (_error != null) ...[
                    const SizedBox(height: Space.lg),
                    InlineBanner(tone: StatusTone.error, live: true, title: _error!),
                  ],
                  const SizedBox(height: Space.xl),
                  BusyButton(
                    label: adding ? 'Add email address' : 'Save',
                    busy: _busy,
                    onPressed: account == null ? null : () => _submit(hasPassword: hasPassword),
                  ),
                ],
              ),
            ),
          ),
        ),
      ],
    );
  }
}
