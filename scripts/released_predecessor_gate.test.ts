import { expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { parse } from 'yaml'

test('release publication waits for an exact published-alpha upgrade proof', () => {
  const release = parse(readFileSync('.github/workflows/release.yml', 'utf8'))
  const gate = release.jobs['released-predecessor-qualification']
  expect(release.jobs.publish.needs).toContain('released-predecessor-qualification')
  expect(gate.needs).toBe('image')
  expect(gate.if).toBeUndefined() // Manual candidates and tag releases both qualify.
  expect(gate['continue-on-error']).toBeUndefined()
  expect(gate.uses).toBe('./.github/workflows/demo-upgrade-qualification.yml')
  expect(gate.with).toMatchObject({
    candidate_image: '${{ needs.image.outputs.image_reference }}',
    candidate_revision: '${{ needs.image.outputs.revision }}',
    final_artifact: true,
    predecessor: 'v0.3.0-alpha.1',
    producer_workflow: 'flidai/leapview/.github/workflows/release.yml',
  })
  const fixture = parse(readFileSync('.github/workflows/demo-upgrade-qualification.yml', 'utf8'))
  expect(fixture.on.workflow_call.inputs.predecessor.default).toBe('schema-32')
  const job = fixture.jobs['historical-transition']
  expect(job.env.LEAPVIEW_HISTORICAL_TRANSITION_PREDECESSOR).toContain('inputs.predecessor')
  expect(job.env.LEAPVIEW_HISTORICAL_TRANSITION_PRODUCER_WORKFLOW).toContain('inputs.producer_workflow')
  const admission = job.steps.find((step: any) => step.id === 'admission')
  expect(admission.with['expected-workflow']).toContain('inputs.producer_workflow')
  const receipt = job.steps.find((step: any) => step.name === 'Upload passed historical transition receipt')
  expect(receipt.with.name).toContain('released-alpha-transition')
  expect(receipt.with.name).toContain('historical-transition')
  expect(receipt.with['if-no-files-found']).toBe('error')
})
