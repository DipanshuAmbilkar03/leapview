import { expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { parse } from 'yaml'

test('publication requires Linux authoring lifecycle on the exact candidate archives', () => {
  const workflow = parse(readFileSync('.github/workflows/release.yml', 'utf8'))
  const job = workflow.jobs['authoring-cli']
  expect(workflow.jobs.publish.needs).toContain('authoring-cli')
  expect(job.if).toBeUndefined()
  expect(job['continue-on-error']).toBeUndefined()
  expect(job.strategy.matrix.include.filter((host: any) => host.os === 'linux')).toEqual([
    { os: 'linux', arch: 'amd64', runner: 'ubuntu-24.04' },
    { os: 'linux', arch: 'arm64', runner: 'ubuntu-24.04-arm' },
  ])
  const steps = job.steps
  const prerequisites = steps.find((step: any) => step.name === 'Install Linux lifecycle keyring dependencies')
  expect(prerequisites).toBeDefined()
  expect(prerequisites.if).toBe("matrix.os == 'linux'")
  expect(prerequisites.run).toContain('dbus gnome-keyring')
  const qualification = steps.find((step: any) => step.name === 'Qualify installed discovery and Linux lifecycle')
  expect(qualification.if).toBeUndefined()
  expect(qualification['continue-on-error']).toBeUndefined()
  expect(qualification.env).toMatchObject({
    RELEASE_TAG: '${{ needs.image.outputs.release_tag }}',
    TARGET_OS: '${{ matrix.os }}',
    TARGET_ARCH: '${{ matrix.arch }}',
  })
  expect(qualification.run).toContain('--archive "$PWD/dist/$package.tar.gz"')
  expect(qualification.run).toContain('--checksum "$PWD/dist/$package.tar.gz.sha256"')
  expect(qualification.run).toContain('--required')
  expect(qualification.run).toContain('if [ "$TARGET_OS" = linux ]; then')
  expect(qualification.run).toContain('--run-lifecycle')
  expect(qualification.run).toContain('--docker-host unix:///var/run/docker.sock')
  expect(qualification.run).toContain('python3 ./deploy/local/qualification/with_keyring.py')
  expect(qualification.run).not.toContain('|| true')
  expect(steps.indexOf(prerequisites)).toBeLessThan(steps.indexOf(qualification))
  expect(steps.indexOf(qualification)).toBeLessThan(steps.findIndex((step: any) => step.name === 'Attest authoring archive provenance'))
  const evidence = steps.find((step: any) => step.name === 'Upload authoring qualification evidence')
  expect(evidence.if).toBe('always()')
  expect(evidence.with.path).toContain('qualification-report.json')
  expect(evidence.with.path).toContain('raw-results.json')
})
