import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../app/providers.dart';
import '../../../app/theme/tokens.dart';
import '../../../core/api/api_failure.dart';
import '../../../core/api/idempotency.dart';
import '../../../core/auth/session_manager.dart';
import '../../../core/formatting/phone.dart';
import '../../../core/widgets/back_office_link.dart';
import '../../../core/widgets/brand_mark.dart';
import '../../../core/widgets/busy_button.dart';
import '../../../core/widgets/content_frame.dart';
import '../../../core/widgets/failure_text.dart';
import '../../../core/widgets/hero_card.dart';
import '../../../core/widgets/inline_banner.dart';
import '../../../core/widgets/status_pill.dart';
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
      // the browser's or phone's password manager offers to save what was just typed
      TextInput.finishAutofillContext();
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
    final wide = widthClassOf(context) == WidthClass.expanded;
    final form = _Form(
      formKey: _form,
      phone: _phone,
      password: _password,
      mode: _mode,
      obscure: _obscure,
      adult: _adult,
      busy: _busy,
      phoneError: _phoneError,
      passwordError: _passwordError,
      formError: _formError,
      formDetail: _formDetail,
      expired: ref.watch(sessionManagerProvider).endReason == SessionEndReason.expired,
      onMode: (m) => setState(() => _mode = m),
      onObscure: () => setState(() => _obscure = !_obscure),
      onAdult: (v) => setState(() => _adult = v),
      onSubmit: _submit,
    );
    return Scaffold(
      body: SafeArea(
        child: wide
            ? Row(
                children: [
                  const Expanded(child: _Panel()),
                  Expanded(
                    child: Center(
                      child: SingleChildScrollView(
                        padding: const EdgeInsets.all(Space.xxl),
                        child: ConstrainedBox(
                          constraints: const BoxConstraints(maxWidth: 420),
                          child: Column(
                            crossAxisAlignment: CrossAxisAlignment.stretch,
                            children: [
                              form,
                              const SizedBox(height: Space.lg),
                              const BackOfficeLink(),
                            ],
                          ),
                        ),
                      ),
                    ),
                  ),
                ],
              )
            : Center(
                child: SingleChildScrollView(
                  padding: const EdgeInsets.all(Space.xl),
                  child: ConstrainedBox(
                    constraints: const BoxConstraints(maxWidth: 420),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.stretch,
                      children: [
                        const Align(alignment: Alignment.centerLeft, child: BrandMark(height: 42)),
                        const SizedBox(height: Space.xxl),
                        form,
                        const SizedBox(height: Space.lg),
                        const BackOfficeLink(),
                      ],
                    ),
                  ),
                ),
              ),
      ),
    );
  }
}

/// The brand panel beside the form on wide screens: the one place the hero surface carries words, not money.
class _Panel extends StatelessWidget {
  const _Panel();

  @override
  Widget build(BuildContext context) {
    final t = Theme.of(context).textTheme;
    final c = context.colors;
    return Padding(
      padding: const EdgeInsets.all(Space.xl),
      child: HeroCard(
        radius: Radii.xxl,
        padding: const EdgeInsets.all(Space.xxxl),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            BrandMark(height: 44, wordmarkColor: c.onHero),
            const Spacer(),
            Text(
              'KES 1 a spin, up to four a day.\nEvery draw on the record.',
              style: t.displaySmall?.copyWith(color: c.onHero),
            ),
            const SizedBox(height: Space.lg),
            Text(
              'Each spin is paid from your M-Pesa, draws use a public randomness beacon, and every prize is paid back to it.',
              style: t.bodyLarge?.copyWith(color: c.onHeroMuted),
            ),
            const Spacer(),
            Text('Play responsibly · 18+', style: t.labelMedium?.copyWith(color: c.onHeroMuted)),
          ],
        ),
      ),
    );
  }
}

class _Form extends StatelessWidget {
  const _Form({
    required this.formKey,
    required this.phone,
    required this.password,
    required this.mode,
    required this.obscure,
    required this.adult,
    required this.busy,
    required this.phoneError,
    required this.passwordError,
    required this.formError,
    required this.formDetail,
    required this.expired,
    required this.onMode,
    required this.onObscure,
    required this.onAdult,
    required this.onSubmit,
  });

  final GlobalKey<FormState> formKey;
  final TextEditingController phone;
  final TextEditingController password;
  final _Mode mode;
  final bool obscure;
  final bool adult;
  final bool busy;
  final String? phoneError;
  final String? passwordError;
  final String? formError;
  final String? formDetail;
  final bool expired;
  final ValueChanged<_Mode> onMode;
  final VoidCallback onObscure;
  final ValueChanged<bool> onAdult;
  final VoidCallback onSubmit;

  @override
  Widget build(BuildContext context) {
    final t = Theme.of(context).textTheme;
    final c = context.colors;
    final creating = mode == _Mode.create;
    return AutofillGroup(
      child: Form(
        key: formKey,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Semantics(
              header: true,
              child: Text(creating ? 'Create your account' : 'Welcome back', style: t.headlineLarge),
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
              const InlineBanner(
                tone: StatusTone.pending,
                icon: Icons.info_rounded,
                live: true,
                title: 'Your session ended. Sign in again to continue.',
              ),
            ],
            const SizedBox(height: Space.xl),
            SegmentedButton<_Mode>(
              segments: const [
                ButtonSegment(value: _Mode.signIn, label: Text('Sign in')),
                ButtonSegment(value: _Mode.create, label: Text('Create account')),
              ],
              selected: {mode},
              showSelectedIcon: false,
              onSelectionChanged: busy ? null : (s) => onMode(s.first),
            ),
            const SizedBox(height: Space.xl),
            TextFormField(
              controller: phone,
              keyboardType: TextInputType.phone,
              autofillHints: const [AutofillHints.telephoneNumber],
              textInputAction: TextInputAction.next,
              inputFormatters: [FilteringTextInputFormatter.allow(RegExp(r'[0-9+ ]'))],
              decoration: InputDecoration(
                labelText: 'M-Pesa phone number',
                helperText: 'For example 0712 345 678',
                errorText: phoneError,
                prefixIcon: const Icon(Icons.phone_iphone_rounded),
              ),
              validator: (v) =>
                  isLikelyKenyanMobile(v ?? '') ? null : 'Enter a Kenyan mobile number such as 0712 345 678.',
            ),
            const SizedBox(height: Space.lg),
            TextFormField(
              controller: password,
              obscureText: obscure,
              autofillHints: [creating ? AutofillHints.newPassword : AutofillHints.password],
              textInputAction: TextInputAction.done,
              onFieldSubmitted: (_) => onSubmit(),
              decoration: InputDecoration(
                labelText: 'Password',
                helperText: creating ? '8 to 128 characters, not a common password, not your number' : null,
                errorText: passwordError,
                prefixIcon: const Icon(Icons.lock_rounded),
                suffixIcon: IconButton(
                  tooltip: obscure ? 'Show password' : 'Hide password',
                  icon: Icon(obscure ? Icons.visibility_rounded : Icons.visibility_off_rounded),
                  onPressed: onObscure,
                ),
              ),
              validator: (v) => (v ?? '').length < 8 ? 'Use at least 8 characters.' : null,
            ),
            if (creating) ...[
              const SizedBox(height: Space.md),
              CheckboxListTile(
                value: adult,
                onChanged: (v) => onAdult(v ?? false),
                controlAffinity: ListTileControlAffinity.leading,
                contentPadding: EdgeInsets.zero,
                title: const Text('I am 18 or older'),
                subtitle: const Text('Your age is checked against your national ID before you can play.'),
              ),
            ],
            if (formError != null) ...[
              const SizedBox(height: Space.lg),
              InlineBanner(tone: StatusTone.error, live: true, title: formError!, message: formDetail),
            ],
            const SizedBox(height: Space.xl),
            BusyButton(label: creating ? 'Create account' : 'Sign in', busy: busy, onPressed: onSubmit),
            const SizedBox(height: Space.xl),
            Text(
              'Play responsibly. Lottery plays cost money and most plays do not win a prize. '
              'You can set deposit limits, take a break or self-exclude at any time from your account.',
              style: t.bodySmall,
            ),
          ],
        ),
      ),
    );
  }
}
