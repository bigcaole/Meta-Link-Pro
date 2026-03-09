import * as AppAPI from '../../bindings/meta-link-pro/backend/app'
import type { GenerateMetaYAMLRequest, ParseReport, ServiceTree, UpdateStatus } from '../types'

export async function parseLinks(input: string): Promise<ParseReport> {
  return await AppAPI.ParseLinks(input) as unknown as ParseReport
}

export async function loadServiceTree(): Promise<ServiceTree[]> {
  return await AppAPI.LoadServiceTree() as unknown as ServiceTree[]
}

export async function generateMetaYAML(payload: GenerateMetaYAMLRequest): Promise<string> {
  return await AppAPI.GenerateMetaYAML(payload as never)
}

export async function exportToDesktop(content: string): Promise<string> {
  return await AppAPI.ExportToDesktop(content)
}

export async function startUpdateCheck(): Promise<UpdateStatus> {
  return await AppAPI.StartUpdateCheck() as unknown as UpdateStatus
}

export async function getUpdateStatus(): Promise<UpdateStatus> {
  return await AppAPI.GetUpdateStatus() as unknown as UpdateStatus
}
