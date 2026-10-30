import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../app/theme/tokens.dart';
import '../../../core/api/api_failure.dart';
import '../../../core/api/idempotency.dart';
import '../../../core/widgets/app_card.dart';
import '../../../core/widgets/async_view.dart';
import '../../../core/widgets/busy_button.dart';
import '../../../core/widgets/content_frame.dart';
import '../../../core/widgets/failure_text.dart';
import '../../../core/widgets/inline_banner.dart';
import '../../../core/widgets/status_pill.dart';
import '../application/account_providers.dart';
import '../domain/account_models.dart';
import 'account_screen.dart';

class KycScreen extends ConsumerStatefulWidget {
  const KycScreen({super.key});

  @override
  ConsumerState<KycScreen> createState() => _KycScreenState();
}

class _KycScreenState extends ConsumerState<KycScreen> {
  final _form = GlobalKey<FormState>();
  final _id = TextEditingController();
  final _name = TextEditingController();
  var _prefilled = false;
  var _busy = false;
  String? _error;
  String? _key;
  String? _sentBody; // the key is reused only for the same details; edited details are a new submission

  @override
  void dispose() {
    _id.dispose();
    _name.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    setState(() => _error = null);
    if (!_form.currentState!.validate()) return;
    setState(() => _busy = true);
    final body = '${_id.text.trim()}|${_name.text.trim()}';
    if (_sentBody != body) _key = null;
    _sentBody = body;
    _key ??= newIdempotencyKey(); // the same key on a retry after a lost answer
    try {
      await ref
          .read(accountRepositoryProvider)
          .submitKyc(nationalId: _id.text.trim(), fullName: _name.text.trim(), idempotencyKey: _key!);
      _key = null;
      ref.invalidate(accountProvider);
    } on ApiFailure catch (f) {
      if (!f.outcomeUncertain) _key = null;
      if (!mounted) return;
      setState(() {
        final d = describeFailure(f);
        _error = f.outcomeUncertain
            ? '${d.title}. Submit again to check: it will not be sent twice.'
            : [d.title, ?d.detail].join('. ');
      });
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final account = ref.watch(accountProvider);
    // start from the names and ID number given at sign-up (owner, 2026-10-30)
    final declared = ref.watch(declaredIdentityProvider).value;
    if (declared != null && !_prefilled) {
      _prefilled = true;
      if (_id.text.isEmpty) _id.text = declared.nationalId;
      if (_name.text.isEmpty) _name.text = declared.fullName;
    }
    final t = Theme.of(context).textTheme;
    final c = context.colors;
    return PageScaffold(
      title: 'Identity verification',
      onRefresh: () async => ref.invalidate(accountProvider),
      children: [
        AsyncView(
          value: account,
          what: 'your verification',
          onRetry: () => ref.invalidate(accountProvider),
          data: (a) {
            final (tone, label) = kycPill(a.kycStatus);
            if (a.kycStatus != KycStatus.none) {
              return InlineBanner(
                tone: tone,
                title: label,
                message: switch (a.kycStatus) {
                  KycStatus.verified => 'Your national ID is verified.',
                  KycStatus.rejected =>
                    'Your ID could not be accepted. Please contact support if you think this is a mistake.',
                  _ => 'Staff check each ID by hand. Pull down to refresh once they have decided.',
                },
              );
            }
            return Form(
              key: _form,
              child: AppCard(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    Row(
                      children: [
                        ExcludeSemantics(child: Icon(Icons.shield_outlined, color: c.primary)),
                        const SizedBox(width: Space.md),
                        Expanded(
                          child: Text(
                            'We verify your Kenyan national ID once, before your first withdrawal. Your name must '
                            'match your ID and the name M-Pesa shows for your number. Your details are stored '
                            'encrypted.',
                            style: t.bodyMedium?.copyWith(color: c.inkMuted),
                          ),
                        ),
                      ],
                    ),
                    const SizedBox(height: Space.xl),
                    TextFormField(
                      controller: _id,
                      keyboardType: TextInputType.number,
                      inputFormatters: [FilteringTextInputFormatter.digitsOnly, LengthLimitingTextInputFormatter(9)],
                      decoration: const InputDecoration(labelText: 'National ID number', helperText: '7 to 9 digits'),
                      validator: (v) =>
                          RegExp(r'^\d{7,9}$').hasMatch(v ?? '') ? null : 'Enter the 7 to 9 digits of your ID number.',
                    ),
                    const SizedBox(height: Space.lg),
                    TextFormField(
                      controller: _name,
                      textCapitalization: TextCapitalization.words,
                      autofillHints: const [AutofillHints.name],
                      decoration: const InputDecoration(
                        labelText: 'Full name',
                        helperText: 'Exactly as on your ID: first and second name',
                      ),
                      validator: (v) => (v ?? '').trim().length < 2 ? 'Enter your full name.' : null,
                    ),
                    if (_error != null) ...[
                      const SizedBox(height: Space.lg),
                      InlineBanner(tone: StatusTone.error, live: true, title: _error!),
                    ],
                    const SizedBox(height: Space.xl),
                    BusyButton(label: 'Submit for verification', busy: _busy, onPressed: _submit),
                  ],
                ),
              ),
            );
          },
        ),
      ],
    );
  }
}
