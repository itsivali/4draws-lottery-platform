import type { AmlCase } from './types'

/** Each monitoring rule's name in words. */
export const ruleLabels: Record<AmlCase['rule'], string> = {
  deposit_velocity: 'Much money in, quickly',
  in_out_little_play: 'Money in and out, little play',
  failed_topups: 'Many failed top-ups',
  shared_id_number: 'ID number on several accounts',
  shared_device: 'Device on several accounts',
  manual: 'Opened by staff',
}
