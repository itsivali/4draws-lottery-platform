import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:sign_in_with_apple/sign_in_with_apple.dart';

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
import '../../../core/widgets/reveal.dart';
import '../../../core/widgets/status_pill.dart';
import '../application/auth_providers.dart';
import '../data/provider_sign_in.dart';
import '../data/sign_in_providers.dart';
import 'email_rules.dart';

enum _Mode { signIn, create }

/// Where the player is: the phone-and-password form (with Google and Apple beside it), or, after Google or Apple
/// answered for an identity that has no account yet, the one question left: their M-Pesa number (ADR 0020).
enum _Step { form, phone }

class SignInScreen extends ConsumerStatefulWidget {
  const SignInScreen({super.key});

  @override
  ConsumerState<SignInScreen> createState() => _SignInScreenState();
}

class _SignInScreenState extends ConsumerState<SignInScreen> {
  final _form = GlobalKey<FormState>();
  final _phone = TextEditingController();
  final _email = TextEditingController();
  final _password = TextEditingController();
  var _mode = _Mode.signIn;
  var _step = _Step.form;
  var _obscure = true;
  var _adult = false;
  var _busy = false;
  var _exchanging = false;
  var _prepared = false;
  ProviderToken? _providerToken;
  StreamSubscription<ProviderToken>? _tokens;
  String? _phoneError, _emailError, _passwordError, _formError, _formDetail;

  @override
  void initState() {
    super.initState();
    _tokens = ref.read(providerSignInProvider).tokens.listen(_providerSignIn);
  }

  @override
  void dispose() {
    _tokens?.cancel();
    _phone.dispose();
    _email.dispose();
    _password.dispose();
    super.dispose();
  }

  void _clearErrors() => _phoneError = _emailError = _passwordError = _formError = _formDetail = null;

  Future<void> _submit() async {
    setState(_clearErrors);
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
              email: _email.text.trim(),
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
          case ProblemFailure(slug: 'invalid-email' || 'email-registered', :final title, :final detail):
            _emailError = detail == null ? title : '$title: $detail';
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

  /// Readies the providers' SDKs once the API has said what it offers (on the web, Google's button needs this first).
  Future<void> _prepare(SignInProviders p) async {
    if (_prepared || !p.any) return;
    _prepared = true;
    try {
      await ref.read(providerSignInProvider).prepare(p);
    } on Exception {
      _prepared = false;
      return;
    }
    if (mounted) setState(() {});
  }

  /// Opens Google's or Apple's own sign-in. The token, if the player went through with it, arrives on the stream.
  Future<void> _open(Future<void> Function() open, String name) async {
    setState(() {
      _clearErrors();
      _busy = true;
    });
    try {
      await open();
    } on Exception {
      if (mounted) setState(() => _formError = '$name could not sign you in. Try again.');
    } finally {
      if (mounted && !_exchanging) setState(() => _busy = false);
    }
  }

  /// A token from Google or Apple: sign in with it, or, for an identity with no account yet, ask for the number.
  Future<void> _providerSignIn(ProviderToken t) async {
    _exchanging = true;
    setState(() {
      _clearErrors();
      _busy = true;
    });
    try {
      final tokens = await ref.read(authRepositoryProvider).signInWithProvider(t, idempotencyKey: newIdempotencyKey());
      await ref.read(sessionManagerProvider).signedIn(tokens);
    } on ApiFailure catch (f) {
      if (!mounted) return;
      setState(() {
        if (f case ProblemFailure(slug: 'sso-not-linked')) {
          _providerToken = t;
          _step = _Step.phone;
          _adult = false;
          _phone.clear();
        } else {
          final d = describeFailure(f);
          _formError = d.title;
          _formDetail = d.detail;
        }
      });
    } finally {
      _exchanging = false;
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _providerSignUp() async {
    final t = _providerToken;
    if (t == null) return;
    setState(_clearErrors);
    if (!_form.currentState!.validate()) return;
    if (!_adult) {
      setState(() => _formError = 'Confirm that you are 18 or older to create an account.');
      return;
    }
    setState(() => _busy = true);
    try {
      final tokens = await ref
          .read(authRepositoryProvider)
          .signUpWithProvider(t, msisdn: _phone.text.trim(), idempotencyKey: newIdempotencyKey());
      await ref.read(sessionManagerProvider).signedIn(tokens);
    } on ApiFailure catch (f) {
      if (!mounted) return;
      setState(() {
        switch (f) {
          case ProblemFailure(slug: 'invalid-msisdn', :final detail):
            _phoneError = detail ?? 'Enter a Kenyan mobile number such as 0712 345 678.';
          case ProblemFailure(slug: 'phone-registered', :final title):
            _formError = title;
            _formDetail = 'Sign in with that number and its password instead.';
          case ProblemFailure(slug: 'invalid-token' || 'sso-already-linked'):
            // the provider's token ran out (they are short-lived) or was used meanwhile: start again
            _back();
            _formError = 'Your ${_providerName(t.provider)} sign-in ran out. Start again.';
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

  void _back() {
    _providerToken = null;
    _step = _Step.form;
    _adult = false;
    _phone.clear();
    _clearErrors();
  }

  static String _providerName(String provider) => provider == 'apple' ? 'Apple' : 'Google';

  @override
  Widget build(BuildContext context) {
    final wide = widthClassOf(context) == WidthClass.expanded;
    final providers = ref.watch(signInProvidersProvider).value ?? SignInProviders.none;
    if (providers.any && !_prepared) unawaited(_prepare(providers));
    final signIn = ref.read(providerSignInProvider);
    final form = _Form(
      formKey: _form,
      phone: _phone,
      email: _email,
      password: _password,
      mode: _mode,
      step: _step,
      obscure: _obscure,
      adult: _adult,
      busy: _busy,
      phoneError: _phoneError,
      emailError: _emailError,
      passwordError: _passwordError,
      formError: _formError,
      formDetail: _formDetail,
      expired: ref.watch(sessionManagerProvider).endReason == SessionEndReason.expired,
      justReset: GoRouterState.of(context).uri.queryParameters['reset'] == '1',
      providers: _prepared ? providers : SignInProviders.none,
      googleButton: _prepared ? signIn.googleButton() : null,
      onMode: (m) => setState(() => _mode = m),
      onObscure: () => setState(() => _obscure = !_obscure),
      onAdult: (v) => setState(() => _adult = v),
      onSubmit: _step == _Step.phone ? _providerSignUp : _submit,
      onGoogle: () => _open(signIn.google, 'Google'),
      onApple: () => _open(() => signIn.apple(providers), 'Apple'),
      onBack: () => setState(_back),
      onForgot: () => context.go('/forgot-password'),
    );
    final heading = _Heading(step: _step, mode: _mode, providerName: _providerToken?.provider);
    final card = _FormCard(child: form);
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
                          constraints: const BoxConstraints(maxWidth: 440),
                          child: Column(
                            crossAxisAlignment: CrossAxisAlignment.stretch,
                            children: [
                              heading.onPage(context),
                              const SizedBox(height: Space.xl),
                              card,
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
            : SingleChildScrollView(
                padding: const EdgeInsets.fromLTRB(Space.lg, Space.sm, Space.lg, Space.xl),
                child: Center(
                  child: ConstrainedBox(
                    constraints: const BoxConstraints(maxWidth: 440),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.stretch,
                      children: [
                        // the sunset header carries the greeting; the form's card rises into its lower edge
                        HeroCard(
                          radius: Radii.xl,
                          padding: const EdgeInsets.fromLTRB(Space.xl, Space.xl, Space.xl, Space.xxxl + Space.lg),
                          child: Column(
                            crossAxisAlignment: CrossAxisAlignment.start,
                            children: [
                              BrandMark(height: 36, wordmarkColor: context.colors.onHero),
                              const SizedBox(height: Space.xl),
                              heading.onHero(context),
                            ],
                          ),
                        ),
                        Transform.translate(offset: const Offset(0, -Space.xxxl), child: card),
                        Transform.translate(offset: const Offset(0, -Space.xl), child: const BackOfficeLink()),
                      ],
                    ),
                  ),
                ),
              ),
      ),
    );
  }
}

/// The greeting (owner's copy, 2026-10-29): a welcome for sign-in, the promise for a new account, and the one question
/// left after Google or Apple. On phones it sits on the sunset header; on wide screens above the card.
class _Heading {
  const _Heading({required this.step, required this.mode, required this.providerName});

  final _Step step;
  final _Mode mode;
  final String? providerName;

  String get title => switch (step) {
    _Step.phone => 'Almost there',
    _Step.form => mode == _Mode.create ? 'Join Tajirika' : 'Good to see you',
  };

  String get lead => switch (step) {
    _Step.phone =>
      'Your ${providerName == 'apple' ? 'Apple' : 'Google'} account is ready to sign in. Add the M-Pesa number you '
          'will play with: it is confirmed by the M-Pesa prompt of your first top-up — no SMS code.',
    _Step.form =>
      mode == _Mode.create
          ? 'Every draw on the record, every prize paid back to your M-Pesa.'
          : 'Sign in to your wallet, your plays and your draws.',
  };

  Widget onHero(BuildContext context) {
    final t = Theme.of(context).textTheme;
    final c = context.colors;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Semantics(
          header: true,
          child: Text(title, style: t.displaySmall?.copyWith(color: c.onHero)),
        ),
        const SizedBox(height: Space.sm),
        Text(lead, style: t.bodyLarge?.copyWith(color: c.onHeroMuted)),
      ],
    );
  }

  Widget onPage(BuildContext context) {
    final t = Theme.of(context).textTheme;
    final c = context.colors;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Semantics(header: true, child: Text(title, style: t.headlineLarge)),
        const SizedBox(height: Space.sm),
        Text(lead, style: t.bodyMedium?.copyWith(color: c.inkMuted)),
      ],
    );
  }
}

/// The form's card: a surface with the float lift and the hero's corner radius, so it reads as one raised sheet.
class _FormCard extends StatelessWidget {
  const _FormCard({required this.child});

  final Widget child;

  @override
  Widget build(BuildContext context) {
    final c = context.colors;
    return Reveal(
      child: DecoratedBox(
        decoration: BoxDecoration(borderRadius: BorderRadius.circular(Radii.xl), boxShadow: Elevation.float(c)),
        child: Material(
          color: c.surface,
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(Radii.xl),
            side: BorderSide(color: c.border),
          ),
          clipBehavior: Clip.antiAlias,
          child: Padding(padding: const EdgeInsets.all(Space.xl), child: child),
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
              'KES 1 a spin, up to four a day.\nFair draws, paid to your M-Pesa.',
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
    required this.email,
    required this.password,
    required this.mode,
    required this.step,
    required this.obscure,
    required this.adult,
    required this.busy,
    required this.phoneError,
    required this.emailError,
    required this.passwordError,
    required this.formError,
    required this.formDetail,
    required this.expired,
    required this.justReset,
    required this.providers,
    required this.googleButton,
    required this.onMode,
    required this.onObscure,
    required this.onAdult,
    required this.onSubmit,
    required this.onGoogle,
    required this.onApple,
    required this.onBack,
    required this.onForgot,
  });

  final GlobalKey<FormState> formKey;
  final TextEditingController phone;
  final TextEditingController email;
  final TextEditingController password;
  final _Mode mode;
  final _Step step;
  final bool obscure;
  final bool adult;
  final bool busy;
  final String? phoneError;
  final String? emailError;
  final String? passwordError;
  final String? formError;
  final String? formDetail;
  final bool expired;
  final bool justReset;
  final SignInProviders providers;
  final Widget? googleButton;
  final ValueChanged<_Mode> onMode;
  final VoidCallback onObscure;
  final ValueChanged<bool> onAdult;
  final VoidCallback onSubmit;
  final VoidCallback onGoogle;
  final VoidCallback onApple;
  final VoidCallback onBack;
  final VoidCallback onForgot;

  @override
  Widget build(BuildContext context) {
    final t = Theme.of(context).textTheme;
    final phoneStep = step == _Step.phone;
    final creating = phoneStep || mode == _Mode.create;
    final phoneField = TextFormField(
      controller: phone,
      keyboardType: TextInputType.phone,
      autofillHints: const [AutofillHints.telephoneNumber],
      textInputAction: phoneStep ? TextInputAction.done : TextInputAction.next,
      onFieldSubmitted: phoneStep ? (_) => onSubmit() : null,
      inputFormatters: [FilteringTextInputFormatter.allow(RegExp(r'[0-9+ ]'))],
      decoration: InputDecoration(
        labelText: 'M-Pesa phone number',
        helperText: 'For example 0712 345 678',
        errorText: phoneError,
        prefixIcon: const Icon(Icons.phone_iphone_rounded),
      ),
      validator: (v) => isLikelyKenyanMobile(v ?? '') ? null : 'Enter a Kenyan mobile number such as 0712 345 678.',
    );
    final adultTile = CheckboxListTile(
      value: adult,
      onChanged: (v) => onAdult(v ?? false),
      controlAffinity: ListTileControlAffinity.leading,
      contentPadding: EdgeInsets.zero,
      title: const Text('I am 18 or older'),
      subtitle: const Text('Your age is checked against your national ID before you can withdraw.'),
    );
    final errorBanner = formError == null
        ? null
        : InlineBanner(tone: StatusTone.error, live: true, title: formError!, message: formDetail);
    return AutofillGroup(
      child: Form(
        key: formKey,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            if (expired && !phoneStep) ...[
              const InlineBanner(
                tone: StatusTone.pending,
                icon: Icons.info_rounded,
                live: true,
                title: 'Your session ended. Sign in again to continue.',
              ),
              const SizedBox(height: Space.xl),
            ],
            if (justReset && !phoneStep) ...[
              const InlineBanner(
                tone: StatusTone.success,
                icon: Icons.check_circle_rounded,
                live: true,
                title: 'Your password is changed. Sign in with it.',
              ),
              const SizedBox(height: Space.xl),
            ],
            if (phoneStep) ...[
              phoneField,
              const SizedBox(height: Space.md),
              adultTile,
              if (errorBanner != null) ...[const SizedBox(height: Space.lg), errorBanner],
              const SizedBox(height: Space.xl),
              BusyButton(label: 'Create account', busy: busy, onPressed: onSubmit),
              const SizedBox(height: Space.sm),
              TextButton(onPressed: busy ? null : onBack, child: const Text('Sign in another way')),
            ] else ...[
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
              phoneField,
              if (creating) ...[
                const SizedBox(height: Space.lg),
                TextFormField(
                  controller: email,
                  keyboardType: TextInputType.emailAddress,
                  autofillHints: const [AutofillHints.email],
                  autocorrect: false,
                  textInputAction: TextInputAction.next,
                  decoration: InputDecoration(
                    labelText: 'Email address',
                    helperText: 'For a link if you ever forget your password',
                    errorText: emailError,
                    prefixIcon: const Icon(Icons.alternate_email_rounded),
                  ),
                  validator: emailFieldError,
                ),
              ],
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
              if (creating) ...[const SizedBox(height: Space.md), adultTile],
              if (errorBanner != null) ...[const SizedBox(height: Space.lg), errorBanner],
              const SizedBox(height: Space.xl),
              BusyButton(label: creating ? 'Create account' : 'Sign in', busy: busy, onPressed: onSubmit),
              if (!creating) ...[
                const SizedBox(height: Space.sm),
                TextButton(onPressed: busy ? null : onForgot, child: const Text('Forgotten your password?')),
              ],
              if (providers.any) ...[
                const SizedBox(height: Space.xl),
                const _OrDivider(label: 'or continue with'),
                const SizedBox(height: Space.lg),
                _ProviderButtons(
                  providers: providers,
                  googleButton: googleButton,
                  busy: busy,
                  onGoogle: onGoogle,
                  onApple: onApple,
                ),
              ],
            ],
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

class _OrDivider extends StatelessWidget {
  const _OrDivider({required this.label});

  final String label;

  @override
  Widget build(BuildContext context) {
    final c = context.colors;
    final line = Expanded(child: Divider(color: c.border));
    return Row(
      children: [
        line,
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: Space.md),
          child: Text(
            label.toUpperCase(),
            style: Theme.of(context).textTheme.labelSmall?.copyWith(color: c.inkMuted, letterSpacing: 1),
          ),
        ),
        line,
      ],
    );
  }
}

/// Google and Apple, each as its provider asks: Google's own rendered button on the web and an outlined one with its
/// mark elsewhere; Apple's black (or, in the dark, white) button with its logo, in the app's own type.
class _ProviderButtons extends StatelessWidget {
  const _ProviderButtons({
    required this.providers,
    required this.googleButton,
    required this.busy,
    required this.onGoogle,
    required this.onApple,
  });

  final SignInProviders providers;
  final Widget? googleButton;
  final bool busy;
  final VoidCallback onGoogle;
  final VoidCallback onApple;

  @override
  Widget build(BuildContext context) {
    final dark = Theme.of(context).brightness == Brightness.dark;
    final appleFg = dark ? Colors.black : Colors.white;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        if (providers.google)
          googleButton != null
              ? SizedBox(height: 48, child: Center(child: googleButton))
              : SizedBox(
                  height: 48,
                  child: OutlinedButton.icon(
                    onPressed: busy ? null : onGoogle,
                    icon: const _GoogleMark(size: 20),
                    label: const Text('Continue with Google'),
                  ),
                ),
        if (providers.google && providers.appleHere) const SizedBox(height: Space.md),
        if (providers.appleHere)
          SizedBox(
            height: 48,
            child: FilledButton.icon(
              onPressed: busy ? null : onApple,
              style: FilledButton.styleFrom(
                backgroundColor: dark ? Colors.white : Colors.black,
                foregroundColor: appleFg,
                disabledBackgroundColor: (dark ? Colors.white : Colors.black).withValues(alpha: 0.4),
                disabledForegroundColor: appleFg.withValues(alpha: 0.7),
              ),
              icon: ExcludeSemantics(
                child: CustomPaint(
                  size: const Size(18, 22),
                  painter: AppleLogoPainter(color: appleFg),
                ),
              ),
              label: const Text('Continue with Apple'),
            ),
          ),
      ],
    );
  }
}

/// Google's four-colour G, drawn (no image to ship, crisp at any size).
class _GoogleMark extends StatelessWidget {
  const _GoogleMark({required this.size});

  final double size;

  @override
  Widget build(BuildContext context) => ExcludeSemantics(
    child: CustomPaint(size: Size.square(size), painter: _GoogleMarkPainter()),
  );
}

class _GoogleMarkPainter extends CustomPainter {
  static const _blue = Color(0xFF4285F4);
  static const _green = Color(0xFF34A853);
  static const _yellow = Color(0xFFFBBC05);
  static const _red = Color(0xFFEA4335);

  @override
  void paint(Canvas canvas, Size size) {
    final stroke = size.width * 0.2;
    final rect = Rect.fromLTWH(stroke / 2, stroke / 2, size.width - stroke, size.height - stroke);
    final paint = Paint()
      ..style = PaintingStyle.stroke
      ..strokeWidth = stroke;
    double deg(double d) => d * 3.141592653589793 / 180;
    for (final (color, start, sweep) in [
      (_blue, 0.0, 40.0),
      (_green, 40.0, 95.0),
      (_yellow, 135.0, 80.0),
      (_red, 215.0, 100.0),
    ]) {
      canvas.drawArc(rect, deg(start), deg(sweep), false, paint..color = color);
    }
    // the bar: from the centre out to the right edge
    canvas.drawRect(
      Rect.fromLTRB(
        size.width / 2,
        size.height / 2 - stroke / 2,
        size.width - stroke / 2 + stroke / 2,
        size.height / 2 + stroke / 2,
      ),
      Paint()..color = _blue,
    );
  }

  @override
  bool shouldRepaint(covariant CustomPainter oldDelegate) => false;
}
