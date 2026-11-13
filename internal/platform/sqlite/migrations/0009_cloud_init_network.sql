ALTER TABLE vm_templates ADD COLUMN cloud_init_network_config TEXT NOT NULL DEFAULT '';
ALTER TABLE vm_templates ADD COLUMN cloud_init_vendor_data TEXT NOT NULL DEFAULT '';
