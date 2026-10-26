import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../app/providers.dart';
import '../../../app/theme/tokens.dart';
import '../../../core/api/api_failure.dart';
import '../../../core/api/idempotency.dart';
import '../../../core/auth/session_manager.dart';
import '../../../core/formatting/phone.dart';
import '../../../core/widgets/brand_mark.dart';
import '../../../core/widgets/busy_button.dart';
import '../../../core/widgets/failure_text.dart';
import '../application/auth_providers.dart';

enum _Mode { signIn, create }

class SignInScreen extends ConsumerStatefulWidget {
  const SignInScreen({super.key});

  @override
  ConsumerState<SignInScreen> createState() => _SignInScreenState();
}

class _SignInScreenState extends ConsumerState<SignInScreen> {
  final _form = GlobalKey<FormState>();
  final _phone = TextEditingController();
  final _password = TextEditingController();
  var _mode = _Mode.signIn;
  var _obscure = true;
  var _adult = false;
  var _busy = false;
  String? _phoneError, _passwordError, _formError, _formDetail;

  @override
  void dispose() {
    _phone.dispose();
    _password.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    setState(() => _phoneError = _passwordError = _formError = _formDetail = null);
    if (!_form.currentState!.validate()) return;
    if (_mode == _Mode.create && !_adult) {
      setState(() => _formError = 'Confirm that you are 18 or older to create an account.');
      return;
    }
    setState(() => _busy = true);
    final repo = ref.read(authRepositoryProvider);
    try {
      final tokens = _mode == _Mode.signIn
          ? await repo.logIn(msisdn: _phone.text.trim(), password: _password.text, idempotencyKey: newIdempotencyKey())
          : await repo.signUp(
              msisdn: _phone.text.trim(),
              password: _password.text,
              idempotencyKey: newIdempotencyKey(),
            );
      await ref.read(sessionManagerProvider).signedIn(tokens);
    } on ApiFailure catch (f) {
      if (!mounted) return;
      setState(() {
        switch (f) {
          case ProblemFailure(slug: 'invalid-msisdn', :final detail):
            _phoneError = detail ?? 'Enter a Kenyan mobile number such as 0712 345 678.';
          case ProblemFailure(slug: 'weak-password', :final title, :final detail):
            _passwordError = detail == null ? title : '$title: $detail';
          default:
            final d = describeFailure(f);
            _formError = d.title;
            _formDetail = d.detail;
        }
      });
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final t = Theme.of(context).textTheme;
    final c = context.colors;
    final expired = ref.watch(sessionManagerProvider).endReason == SessionEndReason.expired;
    final creating = _mode == _Mode.create;
    return Scaffold(
      body: SafeArea(
        child: Center(
          child: SingleChildScrollView(
            padding: const EdgeInsets.all(Space.xl),
            child: ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 420),
              child: AutofillGroup(
                child: Form(
                  key: _form,
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      const Align(alignment: Alignment.centerLeft, child: BrandMark(height: 36)),
                      const SizedBox(height: Space.xxl),
                      Semantics(
                        header: true,
                        child: Text(creating ? 'Create your account' : 'Welcome back', style: t.headlineMedium),
                      ),
                      const SizedBox(height: Space.sm),
                      Text(
                        creating
                            ? 'Use your M-Pesa number. It is confirmed by the M-Pesa prompt of your first top-up — no SMS code.'
                            : 'Sign in with your phone number and password.',
                        style: t.bodyMedium?.copyWith(color: c.inkMuted),
                      ),
                      if (expired) ...[
                        const SizedBox(height: Space.lg),
                        _Notice(
                          icon: Icons.info_rounded,
                          color: c.pending,
                          text: 'Your session ended. Sign in again to continue.',
                        ),
                      ],
                      const SizedBox(height: Space.xl),
                      SegmentedButton<_Mode>(
                        segments: const [
                          ButtonSegment(value: _Mode.signIn, label: Text('Sign in')),
                          ButtonSegment(value: _Mode.create, label: Text('Create account')),
                        ],
                        selected: {_mode},
                        showSelectedIcon: false,
                        onSelectionChanged: _busy ? null : (s) => setState(() => _mode = s.first),
                      ),
                      const SizedBox(height: Space.xl),
                      TextFormField(
                        controller: _phone,
                        keyboardType: TextInputType.phone,
                        autofillHints: const [AutofillHints.telephoneNumber],
                        textInputAction: TextInputAction.next,
                        inputFormatters: [FilteringTextInputFormatter.allow(RegExp(r'[0-9+ ]'))],
                        decoration: InputDecoration(
                          labelText: 'M-Pesa phone number',
                          helperText: 'For example 0712 345 678',
                          errorText: _phoneError,
                          prefixIcon: const Icon(Icons.phone_iphone_rounded),
                        ),
                        validator: (v) =>
                            isLikelyKenyanMobile(v ?? '') ? null : 'Enter a Kenyan mobile number such as 0712 345 678.',
                      ),
                      const SizedBox(height: Space.lg),
                      TextFormField(
                        controller: _password,
                        obscureText: _obscure,
                        autofillHints: [creating ? AutofillHints.newPassword : AutofillHints.password],
                        textInputAction: TextInputAction.done,
                        onFieldSubmitted: (_) => _submit(),
                        decoration: InputDecoration(
                          labelText: 'Password',
                          helperText: creating ? '8 to 128 characters, not a common password, not your number' : null,
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
                      if (creating) ...[
                        const SizedBox(height: Space.md),
                        CheckboxListTile(
                          value: _adult,
                          onChanged: (v) => setState(() => _adult = v ?? false),
                          controlAffinity: ListTileControlAffinity.leading,
                          contentPadding: EdgeInsets.zero,
                          title: const Text('I am 18 or older'),
                          subtitle: const Text('Your age is checked against your national ID before you can play.'),
                        ),
                      ],
                      if (_formError != null) ...[
                        const SizedBox(height: Space.lg),
                        _Notice(icon: Icons.error_rounded, color: c.error, text: _formError!, detail: _formDetail),
                      ],
                      const SizedBox(height: Space.xl),
                      BusyButton(label: creating ? 'Create account' : 'Sign in', busy: _busy, onPressed: _submit),
                      const SizedBox(height: Space.xl),
                      Text(
                        'Play responsibly. Lottery plays cost money and most plays do not win a prize. '
                        'You can set deposit limits, take a break or self-exclude at any time from your account.',
                        style: t.bodySmall,
                      ),
                    ],
                  ),
                ),
              ),
            ),
          ),
        ),
      ),
    );
  }
}

class _Notice extends StatelessWidget {
  const _Notice({required this.icon, required this.color, required this.text, this.detail});

  final IconData icon;
  final Color color;
  final String text;
  final String? detail;

  @override
  Widget build(BuildContext context) {
    final t = Theme.of(context).textTheme;
    return Semantics(
      liveRegion: true,
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          ExcludeSemantics(child: Icon(icon, color: color)),
          const SizedBox(width: Space.md),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(text, style: t.bodyMedium),
                if (detail != null) Text(detail!, style: t.bodySmall),
              ],
            ),
          ),
        ],
      ),
    );
  }
}
