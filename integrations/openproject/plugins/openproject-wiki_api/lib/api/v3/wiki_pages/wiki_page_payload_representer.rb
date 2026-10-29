# frozen_string_literal: true

module API
  module V3
    module WikiPages
      class WikiPagePayloadRepresenter < ::API::Decorators::Single
        include API::Decorators::LinkedResource
        include API::Decorators::FormattableProperty

        def self.create(*args, update: false, **kwargs)
          new(*args, update:, **kwargs)
        end

        def initialize(represented, current_user:, update: false, embed_links: false)
          @update = update
          super(represented, current_user:, embed_links:)
        end

        property :title

        formattable_property :text

        property :lock_version,
                 render_nil: true

        associated_resource :parent,
                            v3_path: :wiki_page,
                            representer: ::API::V3::WikiPages::WikiPageListItemRepresenter,
                            link_title_attribute: :title,
                            skip_link: ->(*) { represented.try(:parent_id).nil? },
                            getter: ->(*) {}

        include ::API::Utilities::PayloadRepresenter

        def from_hash(hash, *)
          hash ||= {}
          reject_unwritable_properties!(hash)
          super
        end

        def _type
          "WikiPage"
        end

        private

        def update_payload?
          @update
        end

        def reject_unwritable_properties!(hash)
          unwritable = %w[protected slug author wiki]
          unwritable << "title" if update_payload?

          unwritable.each do |name|
            raise_unwritable(camel(name)) if hash.key?(name) || hash.key?(camel(name))
          end

          links = hash["_links"]
          return unless links.is_a?(Hash)

          %w[author wiki].each do |rel|
            raise_unwritable(rel) if links.key?(rel)
          end
          raise_unwritable("title") if update_payload? && links.key?("title")
        end

        def camel(name)
          name.to_s.camelize(:lower)
        end

        def raise_unwritable(property)
          raise ::API::Errors::UnwritableProperty.new(
            property,
            "The property '#{property}' is not writable on this resource."
          )
        end
      end
    end
  end
end
