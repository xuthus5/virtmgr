ALTER TABLE vm_metadata ADD COLUMN folder TEXT NOT NULL DEFAULT '';
ALTER TABLE vm_metadata ADD COLUMN resource_group TEXT NOT NULL DEFAULT '';

CREATE INDEX vm_metadata_folder_idx ON vm_metadata(folder);
CREATE INDEX vm_metadata_resource_group_idx ON vm_metadata(resource_group);
