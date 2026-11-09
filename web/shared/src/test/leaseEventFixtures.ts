import type { JobLeaseEvent } from '../types';
export const preciseTime = '2026-11-08T05:00:00.123456789Z';
export function leaseEvent(eventId = '900719925474099312345678901234567890', overrides: Partial<JobLeaseEvent> = {}): JobLeaseEvent {
  return {
    eventId, eventType: 'lease_renewed', occurredAt: preciseTime,
    jobKey: { tenantId: 'tenant', jobId: 'job' }, cell: '<img src=x onerror=alert(1)>', cellId: 'historical-cell', cellRepo: 'github.com/old/repo',
    goalId: 'primary-goal', jobType: 'original-type', route: { jobType: 'resume', taskType: 'task:execute' },
    leaseId: 'historical-lease', workerId: '<script>window.hacked=true</script>',
    leaseExpiresAt: '2026-11-08T05:01:00.987654321Z', previousLeaseExpiresAt: '2026-11-08T06:00:00.999999999Z', ...overrides,
  };
}
